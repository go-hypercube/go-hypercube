// Package queue defines the portable job-transport contract used by the
// framework and implemented by driver packages. It has no dependency on
// the rest of the framework.
//
// # Driver Authoring Guidelines
//
// When implementing the Queue interface for a new backend, driver authors
// must adhere to the following operational behaviors:
//
// ## Explicit Dead-Lettering vs Automatic Broker Redrive
//
// Explicit Queue.DeadLetter is the portable, framework-controlled path for
// permanently failed messages. A driver MUST NOT independently introduce an
// application retry limit or decide that a message has exhausted its retries.
// Where the driver controls broker provisioning and the broker permits it,
// broker-managed automatic redrive SHOULD be disabled by default so it does not
// compete with the caller's retry policy.
//
// Some deployments enable broker-managed redrive as a transport-level safety
// mechanism, some brokers require it, and externally provisioned resources may
// be outside the driver's control. This is permitted, but it is outside the
// explicit DeadLetter transition. A driver with unavoidable or enabled
// automatic redrive MUST document that behavior. Where receive-count limits are
// configurable, operators SHOULD set them above the application's retry limits
// so automatic redrive acts as crash-loop protection rather than normal retry
// policy.
//
// Driver authors must distinguish between these two modes:
//   - Automatic Broker Redrive: the broker moves a message according to broker
//     configuration, without an explicit DeadLetter call. It may preserve only
//     the original published state and its receive count may not exactly match
//     Message.Attempt.
//   - Queue.DeadLetter (Explicit Call): the driver executes the caller's
//     decision and MUST preserve the Message state supplied by that call.
//
// Because callers may mutate Payload or Extra before calling DeadLetter, a
// driver for an immutable-message broker MUST NOT use a native NACK or reject
// operation when that would dead-letter the original, unmodified message.
// Instead, it must:
//  1. Publish the updated Message to the dead-letter target.
//  2. Acknowledge or delete the original message from the source queue.
//  3. Publish first and delete second to preserve at-least-once safety across
//     the non-atomic dual write.
package queue

import (
	"context"
	"time"
)

// Queue is the storage contract for a job queue driver.
//
// # Responsibilities
//
// A Queue implementation is a DUMB EXECUTOR. It stores messages,
// delivers them, and performs exactly the state transition it is told
// to perform. It holds NO opinions about retry policy, attempt
// limits, or when a message is "permanently failed" — all decisions
// belong to the caller (the app/manager layer), which knows each
// job's individual retry policy.
//
// # Message ownership
//
// Message pointers and their mutable fields MUST NOT be shared across an
// enqueue or delivery boundary.
//
// Before calling Push, the caller owns the Message and may initialize all
// caller-controlled fields. While Push is executing, the caller MUST NOT read
// or mutate the Message or its Payload and Extra byte slices. Before Push
// reports success for an item, the driver MUST snapshot that item, including
// independent copies of Payload and Extra; it MUST NOT retain the caller's
// *Message or aliases to caller-owned mutable memory. If ID is empty, the
// driver assigns it on the caller's Message before taking the snapshot. After
// Push returns, the caller retains ownership of its original Message and may
// reuse or modify it without affecting the enqueued message.
//
// Every successful Pop transfers ownership of a new, independently allocated
// *Message to the caller for that delivery. Its Payload and Extra MUST NOT
// alias driver-owned storage or any Message returned by an earlier delivery.
// While processing that delivery, the caller MAY modify Payload, Extra, and
// Delay. The caller MUST NOT modify ID, QueueName, Namespace, JobName, Attempt,
// VisibilityTimeout, or DriverData. This is a caller precondition: if the caller
// modifies a protected field, subsequent behavior for that delivery is
// unspecified. A driver MAY reject such a call with ErrInvalidArgument or
// ErrNotFound, but is not required to detect, repair, or compensate for the
// caller's mutation.
//
// The caller MUST give the driver exclusive access to a delivery while Ack,
// Retry, DeadLetter, or ExtendVisibility is executing and MUST NOT concurrently
// read or mutate the Message or its Payload and Extra. Retry and DeadLetter
// MUST snapshot any caller-modifiable fields they preserve before reporting
// success for that item.
//
// After Ack, Retry, or DeadLetter reports success for an item, the caller MUST
// stop accessing that delivery and all memory reachable through it. If a batch
// operation partially fails, ownership is relinquished only for successful
// items; the caller retains ownership of failed items. ExtendVisibility does
// not transfer ownership back to the driver after the call returns.
//
// # Message life-cycle / state transitions
//
//	Push             -> pending
//	Pop              -> pending -> in-flight      (Attempt incremented, Delay reset)
//	Ack              -> in-flight -> done         (removed | terminal)
//	Retry            -> in-flight -> pending      (re-added)
//	DeadLetter       -> in-flight -> failed       (removed | terminal)
//	ExtendVisibility -> in-flight -> in-flight    (optional; visibility reset)
//
// There is no conditional transition and no "last attempt" — the
// driver never decides that a message is exhausted. If the caller
// wants a message gone, it calls Ack or DeadLetter.
//
// # Delivery semantics
//
// Delivery is AT-LEAST-ONCE within each driver's documented durability
// guarantees. A driver MAY redeliver a message that is still in-flight (e.g.
// after a visibility timeout or worker crash). Handlers MUST therefore be
// idempotent.
//
// ## Delivery identity and settlement
//
// Every successful Pop creates a distinct delivery of a logical message.
// The driver MUST return an independently owned *Message for each delivery
// and MUST attach delivery-specific state to Message.DriverData (for example,
// an SQS receipt handle or a lease token). The driver MUST NOT mutate a
// *Message returned by an earlier Pop when the logical message is redelivered.
//
// Ack, Retry, DeadLetter, and ExtendVisibility settle or modify the specific
// delivery represented by DriverData; Message.ID alone is not sufficient. This
// delivery-token requirement does not require a driver to defend against a
// caller that modified fields the ownership contract requires it to preserve.
//
// A driver MUST verify that the delivery is still current before applying the
// operation. If its visibility expired and another worker has claimed the
// message, or if it was already settled, the operation MUST make no state
// change and MUST return ErrNotFound for that item.
//
// ## Delivery guarantees: at-least-once
//
// After Push reports success for an item, the driver MUST retain its snapshot
// according to the driver's documented durability guarantees until the message
// is Acked, Retried, or DeadLettered. Delivery may occur more than once. A
// driver MUST NOT provide at-most-once behavior by silently removing a message
// merely because it was popped.
//
// ### Why duplicates happen
//
// A worker crashes (or loses connection) *after* doing the work but
// *before* sending the ack:
//
//	Worker                        Queue
//	 │ pop ✓                        │
//	 │ ... runs the job ...         │
//	 │            ✗ CRASH           │  ← ack never sent
//	                                │ message still "in-flight"
//	                                │ → redelivered to another worker
//
// The queue cannot tell "job finished" from "worker died", so it must
// redeliver. Result: the job may run twice.
//
// ### Rule for handler authors
//
// Handlers MUST be idempotent: running twice must have the same effect
// as running once.
//
// # Batch Operations & Error Handling
//
// Methods that accept variadic `msgs ...*Message` (Push, Ack, Retry, DeadLetter)
// operate as batch requests. An empty batch is a successful no-op: the method
// MUST return nil without contacting the backend, even if ctx is already done.
//
// In a non-empty batch, a nil *Message is an individual item failure with
// ErrInvalidArgument. The driver MUST NOT panic or send that item to the backend
// and MUST continue processing every non-nil item. Its BatchError entry retains
// the nil Msg and its Index from the original input.
//
// Drivers MUST handle chunking internally to abstract backend limits away from
// the framework. If the caller passes 500 messages, but the underlying broker
// (e.g., AWS SQS) only supports batches of 10, the driver is responsible for
// splitting the slice, executing the 50 API calls (concurrently or sequentially),
// and aggregating the results.
//
// If one, some, or all items fail, the driver MUST return a single *BatchError
// containing every individual item failure while allowing independent successful
// items to complete. This includes a one-item batch; item-level errors such as
// ErrInvalidArgument and ErrNotFound are stored in ItemError.Err rather than
// returned directly. Errors for the batch operation as a whole may be returned
// directly when they cannot be attributed to individual items.
//
// # Context cancellation
//
// Every method in this package's interfaces MUST observe ctx. If an operation
// has not completed when ctx ends, the method MUST stop waiting and return
// ctx.Err() directly or an error that wraps it so callers can use errors.Is.
// Drivers SHOULD stop issuing new backend work promptly after cancellation.
// The empty-batch no-op described above is the exception: it returns nil without
// consulting ctx because there is no operation to cancel.
//
// A context error does not prove that the operation had no effect. Work already
// submitted to a remote backend may complete before, during, or after
// cancellation even though the driver returns a context error. Callers MUST
// treat the outcome as unknown and use message IDs, settlement semantics, or
// backend reconciliation rather than assuming the operation did not happen.
//
// # Concurrency
//
// Implementations MUST be safe for concurrent use by multiple goroutines.
type Queue interface {
	// Push enqueues one or more messages into their respective QueueName targets.
	// Each successfully enqueued message is snapshotted according to the Message
	// ownership rules above.
	//
	// ID identifies a logical message and MUST be globally unique across all
	// queues within the driver instance or configured isolation namespace. For
	// each message without an ID, the driver MUST assign a collision-resistant ID
	// on the caller's Message and preserve it in the snapshot. A caller-supplied
	// ID asserts the same uniqueness requirement.
	//
	// Push is not idempotent. If an ID already belongs to a message currently
	// retained by the driver, Push MUST fail the duplicate item with
	// ErrAlreadyExists and MUST NOT overwrite, replace, or modify the existing
	// message. Within one batch. Drivers are not required to retain
	// tombstones solely to detect reuse after a message has been permanently
	// removed.
	//
	// Push does not validate queue names, namespaces, or jobs — the caller
	// is responsible for validation prior to enqueueing.
	//
	// Errors:
	//   - *BatchError: if one or more messages failed to enqueue. A duplicate ID
	//     has ErrAlreadyExists in its ItemError.Err.
	Push(ctx context.Context, msgs ...*Message) error

	// Pop retrieves up to maxMessages visible messages from the named queue and
	// moves them to in-flight state. maxMessages MUST be greater than zero;
	// otherwise Pop returns (nil, ErrInvalidArgument).
	//
	// Pop is a blocking operation when no message is currently visible. The
	// driver MUST wait until at least one message becomes visible, ctx ends, or
	// the driver's configured polling timeout expires. A driver MAY implement
	// this with backend-native long polling or with its own polling loop; that is
	// an implementation detail. A backend that returns an empty response before
	// the configured timeout does not permit the driver to return early. The
	// driver MUST continue waiting or polling for the remainder of the period.
	// Polling intervals and timeouts are driver configuration concerns exposed
	// when that driver is constructed.
	//
	// Returning between one and maxMessages messages is a successful result even
	// when fewer than maxMessages are available. Before returning each message,
	// the driver MUST increment msg.Attempt and reset msg.Delay to zero. This
	// prevents the delay used by Push or a previous Retry from being reused
	// implicitly by a later Retry.
	//
	// If the configured polling timeout expires without a visible message, Pop
	// returns (nil, ErrEmpty). Pop MUST NOT return ErrEmpty before that timeout
	// expires. Callers MUST treat ErrEmpty as a normal empty condition, not an
	// infrastructure failure. If ctx ends first, Pop returns (nil, ctx.Err()) or
	// an error wrapping ctx.Err(). A message already visible when Pop begins may
	// be returned immediately.
	//
	// Pop MUST NOT return both delivered messages and a non-nil error, and MUST
	// NOT return an empty slice with a nil error. Message ordering is
	// driver-dependent; callers MUST NOT rely on FIFO or any other ordering unless
	// a concrete driver explicitly provides a stronger guarantee.
	Pop(ctx context.Context, queueName string, maxMessages int) ([]*Message, error)

	// Ack permanently removes one or more in-flight deliveries, marking their
	// logical messages as successfully processed. Each delivery is identified
	// by its driver-owned DriverData; ID alone MUST NOT be used for settlement.
	//
	// Ack is a command, not a decision: the caller determined that the
	// messages succeeded.
	//
	// Errors:
	//   - *BatchError: if one or more messages failed to acknowledge. An item
	//     that does not identify a current in-flight delivery has ErrNotFound.
	Ack(ctx context.Context, msgs ...*Message) error

	// Retry returns one or more in-flight deliveries to the pending state after
	// each message's current Delay, measured from the time Retry is applied to
	// that item. Because Pop resets Delay to zero, retry is immediate unless the
	// caller explicitly sets a new Delay before calling Retry. Each delivery is
	// identified by its driver-owned DriverData; ID alone MUST NOT be used for
	// settlement.
	//
	// Retry never dead-letters and never enforces an attempt limit — retry
	// policy belongs entirely to the caller, which reads msg.Attempt and
	// applies its own per-job policy.
	//
	// Retry is a command, not a decision: the caller already decided these
	// attempts failed and wants another attempt scheduled.
	//
	// Errors:
	//   - *BatchError: if one or more messages failed to re-queue. An item that
	//     does not identify a current in-flight delivery has ErrNotFound.
	Retry(ctx context.Context, msgs ...*Message) error

	// DeadLetter permanently removes one or more in-flight deliveries from the
	// primary queue, marking their logical messages as permanently failed. Each
	// delivery is identified by its driver-owned DriverData; ID alone MUST NOT
	// be used for settlement. The messages MUST NOT be redelivered to standard
	// workers.
	//
	// Drivers with a native dead-letter facility (e.g. an SQS DLQ, Redis
	// "dead" list, or a failed-messages table) SHOULD park messages there
	// for inspection, preserving all message fields—including any updates
	// made to msg.Extra or msg.Payload prior to calling DeadLetter.
	//
	// Note for Immutable Message Brokers (SQS, Pub/Sub, RabbitMQ, Kafka):
	// Calling DeadLetter is an explicit command containing updated message state.
	// Native broker NACK/reject mechanisms usually route the ORIGINAL un-mutated
	// message to a DLQ. Drivers for immutable brokers MUST therefore publish
	// the updated message to the dead-letter target and then acknowledge/delete
	// the original message from the source queue.
	//
	// DeadLetter does not accept an error or cause parameter. If callers
	// need to record why a message failed, they attach failure details to
	// msg.Extra before calling DeadLetter.
	//
	// DeadLetter is a command, not a decision: the caller — not the driver —
	// determined that the message is permanently failed. Drivers MUST NOT invent
	// or enforce application retry limits or TTL-based failure policy. Separately
	// configured or unavoidable broker-managed redrive may still occur as the
	// transport-level behavior described in the package documentation.
	//
	// Errors:
	//   - *BatchError: if one or more messages failed to dead-letter. An item
	//     that does not identify a current in-flight delivery has ErrNotFound.
	DeadLetter(ctx context.Context, msgs ...*Message) error
}

// Message is a unit of work traveling through a queue.
type Message struct {
	// ID identifies this logical message and is globally unique across all queues
	// within a driver instance or configured isolation namespace. Push assigns a
	// collision-resistant ID when this field is empty. A caller that supplies an
	// ID is responsible for satisfying the same uniqueness requirement. IDs MUST
	// NOT be reused while the original message is retained by the driver.
	ID string

	// QueueName is the queue this message belongs to. The caller sets it before
	// Push and MUST NOT modify it after Pop.
	QueueName string

	// Namespace and JobName identify which registered job this
	// message is for. The queue driver treats them as opaque.
	Namespace string
	JobName   string

	// Delay determines how long the message waits before becoming visible. Push
	// applies it from the time the message is enqueued. Pop resets it to zero on
	// every delivery. To defer a Retry, the caller MUST set a new Delay before
	// calling Retry; leaving it zero makes the retry immediately eligible.
	Delay time.Duration

	// Payload is the application-specific data carried by the message. Queue
	// drivers treat this field as opaque bytes and never inspect its contents.
	// Drivers MUST copy it at enqueue and delivery ownership boundaries. The
	// caller MAY replace or mutate it while exclusively owning a delivery.
	Payload []byte

	// Extra holds opaque auxiliary metadata attached to the message. It is
	// intended for information about the message rather than the payload itself,
	// such as failure annotations, routing hints, or correlation IDs. Queue
	// drivers treat its contents as opaque and MUST copy it at enqueue and
	// delivery ownership boundaries. The caller MAY replace or mutate it while
	// exclusively owning a delivery.
	Extra []byte

	// Attempt is the number of times this message has been popped
	// for delivery. The first delivery has Attempt == 1.
	//
	// Ownership: the DRIVER owns this counter — it MUST increment
	// Attempt on every Pop. The CALLER (the app/manager layer) owns
	// the DECISION of what to do with the count (retry policy,
	// max-attempts limit, dead-lettering). Drivers MUST NOT enforce
	// any attempt limit of their own.
	Attempt int

	// VisibilityTimeout is how long this message stays invisible to
	// other workers after being popped. If the worker has not Acked,
	// Retried, or DeadLettered the message within this window, the
	// driver MAY make it visible again (redelivery — see the
	// at-least-once guarantee).
	//
	// Ownership: the CALLER sets this per message or based on the
	// job's configured max runtime. If zero, the driver applies its
	// own default.
	VisibilityTimeout time.Duration

	// DriverData is private, delivery-specific metadata reserved exclusively for
	// driver implementations (e.g. an SQS ReceiptHandle, RabbitMQ DeliveryTag,
	// or Redis lease token). Drivers MUST replace it for every delivery and use
	// it to reject stale settlement or visibility operations; ID alone does not
	// identify a delivery.
	//
	// Callers MUST preserve this value unchanged between Pop and Ack, Retry,
	// DeadLetter, or ExtendVisibility, and MUST NOT inspect, modify, or depend on
	// it. A driver MUST ignore DriverData supplied to Push, MUST NOT persist it as
	// message data, and MUST replace it with valid delivery state on every Pop.
	DriverData any
}

// VisibilityExtender is an OPTIONAL interface implemented by drivers whose
// in-flight deliveries have visibility deadlines that can be changed. Drivers
// that keep deliveries in-flight indefinitely do not need to implement it.
//
// Long-running workers use this capability for heartbeating so a delivery does
// not become visible to another worker while it is still being processed.
type VisibilityExtender interface {
	// ExtendVisibility resets the visibility deadline for a specific in-flight
	// delivery to approximately the time the driver applies this call plus
	// extension. extension is a new visibility duration measured from now,
	// not a duration added to the existing deadline.
	//
	// The delivery is identified by its driver-owned DriverData; ID alone MUST
	// NOT be used to select or update it. If the delivery is stale—including when
	// its visibility deadline expired, another worker claimed it, or it was
	// already Acked, Retried, or DeadLettered—the driver MUST make no state change
	// and return ErrNotFound.
	//
	// Errors:
	//   - ErrInvalidArgument: extension is less than or equal to zero.
	//   - ErrNotFound: the delivery is stale or no longer in flight.
	ExtendVisibility(ctx context.Context, extension time.Duration, msg *Message) error
}

// RedriveProvider is an OPTIONAL interface implemented by drivers that support
// programmatically moving messages from a dead-letter store back into the
// active queue.
type RedriveProvider interface {
	// Redrive transfers up to count messages from the dead-letter store for
	// queueName back into the active pending queue for queueName. count MUST be
	// greater than zero; otherwise Redrive returns (0, ErrInvalidArgument).
	//
	// Returning fewer than count is normal. An empty dead-letter store returns
	// (0, nil). Message selection and ordering are driver-dependent unless a
	// concrete driver explicitly provides a stronger guarantee.
	//
	// Redrive preserves each logical message's ID, QueueName, Namespace, JobName,
	// Payload, Extra, and VisibilityTimeout. It resets Attempt and Delay to zero
	// and clears DriverData before making the message immediately eligible in the
	// active queue. The next Pop therefore returns Attempt == 1 with new
	// delivery-specific DriverData.
	//
	// n is the number of messages for which the transfer fully completed. If an
	// error occurs after a partial transfer, Redrive returns the completed count
	// together with the error.
	//
	// When the backend cannot move a message atomically, the driver MUST publish
	// it to the active queue before removing it from the dead-letter store. A
	// failure between those operations may leave the same logical message in both
	// stores. This intentional at-least-once tradeoff prevents message loss;
	// callers must use ID to detect or reconcile duplicates.
	Redrive(ctx context.Context, queueName string, count int) (n int, err error)
}

// StatsProvider is an OPTIONAL interface. A Queue driver that can
// cheaply report queue state implements it; the framework treats it
// as unavailable otherwise.
type StatsProvider interface {
	// Queues returns the names of all queues that currently hold
	// at least one message. Empty queues MAY be omitted — absence
	// from this list does not mean the queue "doesn't exist".
	Queues(ctx context.Context) ([]string, error)

	// Stats returns a snapshot for the named queue. On success, the returned
	// QueueStats.Name MUST equal queueName.
	//
	// Queue names are logical and need not be provisioned explicitly. For an
	// unknown or empty queue, Stats MUST return
	// (QueueStats{Name: queueName}, nil). On error, callers MUST ignore the
	// returned snapshot; drivers SHOULD return the zero QueueStats value.
	Stats(ctx context.Context, queueName string) (QueueStats, error)
}

// QueueStats is a point-in-time snapshot of a single queue's state —
// the queue named in Name. All values are APPROXIMATE: they reflect
// what the driver could observe at snapshot time and MAY be stale
// the moment they are returned (concurrent workers, replication lag,
// eventual consistency).
//
// The categories describe distinct logical states, but backend snapshots may
// temporarily overlap or omit messages because of eventual consistency and
// races during state transitions. Their sum is not an exact queue total.
//
// Callers MUST NOT use QueueStats for correctness decisions (e.g. "if Ready,
// Delayed, and InFlight are all zero, the job is done") — only for
// observability, alerting, and coarse back-pressure heuristics.
type QueueStats struct {
	// Name is the name of the queue this snapshot describes. All
	// counts below are scoped to THIS queue only — they never
	// aggregate across queues.
	Name string

	// Ready is the approximate number of messages visible and eligible for
	// immediate delivery.
	//
	// Where the backend can distinguish lease expiry, a message whose visibility
	// lease expired counts as Ready rather than InFlight until it is redelivered.
	// Eventually consistent backends MAY briefly report it in both categories or
	// in neither category during the transition.
	Ready int64

	// Delayed is the approximate number of pending messages not yet eligible for
	// delivery because their Push or Retry delay has not elapsed. It does not
	// include messages hidden by an active visibility lease.
	Delayed int64

	// InFlight is the approximate number of delivered messages currently hidden
	// by an active visibility lease and not yet Acked, Retried, or DeadLettered.
	// An expired lease is no longer logically InFlight, although a backend MAY
	// report it temporarily because statistics are approximate.
	InFlight int64

	// Dead is the approximate number of dead-lettered messages
	// belonging to THIS queue, if the driver keeps them (see
	// Queue.DeadLetter). Drivers that discard dead-lettered
	// messages report 0.
	Dead int64
}
