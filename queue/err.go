package queue

import (
	"errors"
	"fmt"
)

// ErrEmpty is returned by Pop when queueName currently has no visible
// message available. All drivers MUST return this exact error
// (checkable via errors.Is) instead of a driver-specific empty error.
var ErrEmpty = errors.New("queue: no message available")

// ErrInvalidArgument is returned when an operation receives an argument that
// violates the Queue contract. Drivers MUST return this error directly or wrap
// it so callers can detect it with errors.Is.
var ErrInvalidArgument = errors.New("queue: invalid argument")

// ErrAlreadyExists is an item-level Push error indicating that the supplied
// Message.ID already identifies a message retained by the driver. Push is not
// idempotent: the existing message remains unchanged and the duplicate item is
// not enqueued. This error is reported through ItemError.Err in a BatchError.
var ErrAlreadyExists = errors.New("queue: message already exists")

// ErrNotFound is returned by Ack, Retry, DeadLetter, and ExtendVisibility
// when no in-flight message matches the given ID or driver state.
//
// This can happen when the message was already acked, retried, or
// dead-lettered, or if the visibility timeout expired and another worker
// claimed ownership.
//
// Drivers MUST return this error (directly or wrapped via errors.Is)
// for missing in-flight messages so callers can distinguish "message
// already handled/lost" from infrastructure failures.
var ErrNotFound = errors.New("queue: message not found")

// ItemError identifies a specific message that failed within a batch operation.
type ItemError struct {
	// Index is the zero-based position of the message in the original input arguments.
	Index int

	// Msg points to the original Message instance passed to the batch call.
	Msg *Message

	// Err is the specific failure reason for this item.
	Err error
}

// BatchError is returned when one or more items in a batch operation fail.
// Callers should inspect item-level failures using errors.As:
//
//	var batchErr *queue.BatchError
//	if errors.As(err, &batchErr) {
//	    for _, item := range batchErr.Failed {
//	        log.Printf("Item %d failed: %v", item.Index, item.Err)
//	    }
//	}
type BatchError struct {
	// Failed lists each item that failed along with its error details.
	Failed []ItemError
}

func (e *BatchError) Error() string {
	return fmt.Sprintf("batch operation failed for %d item(s)", len(e.Failed))
}
