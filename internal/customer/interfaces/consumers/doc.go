// Package consumers is the customer context's message-consumer delivery adapter:
// long-running subscribers that turn inbound bus messages into application
// commands (the mirror image of interfaces/http).
//
// The in-process wiring registers handlers directly on the bus in Module (see
// infrastructure/messaging.InboundHandlers). When moving to Kafka, a consumer
// process defined here would own the consumer-group loop, decode the message,
// build the command, and call the application handler — with ret/dead-letter
// policy and offset commits handled at this layer.
package consumers
