package eventbus

// Subscriber registers handlers for named event types. Registration happens once
// at composition time (see each context's RegisterSubscriptions).
type Subscriber interface {
	// Subscribe binds handler to every event whose Name equals eventName.
	Subscribe(eventName string, handler Handler) error
}
