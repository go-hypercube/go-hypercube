package job

// QueueNamed is optional. A job that doesn't implement it runs on the
// "default" queue.
type QueueNamed interface {
	QueueName() string
}

// QueueNameFor returns j's declared queue name, or "default" if j
// doesn't implement QueueNamed or returns an empty string.
func QueueNameFor(j Job) string {
	if qn, ok := j.(QueueNamed); ok && qn.QueueName() != "" {
		return qn.QueueName()
	}
	return "default"
}
