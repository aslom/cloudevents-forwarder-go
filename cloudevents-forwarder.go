package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/kelseyhightower/envconfig"
)

type ForwardingAction struct {
	client cloudevents.Client

	MyName string `envconfig:"NAME" default:"CloudEventsForwarder"`

	SleepSeconds float64 `envconfig:"CLOUDEVENTS_FORWARDER_SLEEP_SECONDS" default:"0"`

	// If the K_SINK environment variable is set, then events are sent there,
	// otherwise we simply reply to the inbound request.
	Target string `envconfig:"CLOUDEVENTS_FORWARDER_TARGET" required:"true"`

	// Port on which to listen for cloudevents
	Port int `envconfig:"PORT" default:"8080"`

	//LogLevel int `envconfig:"LOG_LEVEL" default:"info"`

	PrintEvent bool `envconfig:"PRINT_EVENT" default:"false"`
}

// provide status when checking action from browser
func (action *ForwardingAction) handleGet(rw http.ResponseWriter, req *http.Request) {
	log.Printf("handling GET")
	fmt.Fprintf(rw, "Hello from %s\n", action.MyName)
}

// ReceiveAndReply is invoked whenever we receive an event.
func (action *ForwardingAction) ReceiveAndReply(ctx context.Context, event cloudevents.Event) cloudevents.Result {
	//log.Printf("Forwarder called")
	// TODO sleep seconds here or in receiver?
	//log.Printf("forwarding event from source %s id %s", event.Context.GetSource(), event.Context.GetID())

	if err := action.forwardEvent(ctx, event); err != nil {
		return cloudevents.NewHTTPResult(400, "failed to forward event: %s", err)
	}
	return cloudevents.NewHTTPResult(200, "OK")
}

func (action *ForwardingAction) forwardEvent(ctx context.Context, event cloudevents.Event) cloudevents.Result {
	if action.PrintEvent {
		log.Printf("event %v", event)
	}
	if action.Target == "print" {
		return nil
	}
	ctx = cloudevents.ContextWithTarget(ctx, action.Target)
	ctx = cloudevents.WithEncodingStructured(ctx)
	retryCtx := cloudevents.ContextWithRetriesExponentialBackoff(ctx, 10*time.Millisecond, 10)
	result := action.client.Send(retryCtx, event)
	log.Printf("sleeping %f seconds after forwarding event from source %s id %s with result %q", action.SleepSeconds, event.Context.GetSource(), event.Context.GetID(), result)
	if action.SleepSeconds > 0 {
		milliseconds := action.SleepSeconds * 1000
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	}
	if cloudevents.IsACK(result) {
		return nil
	}
	return result
}

func main() {
	action := ForwardingAction{}
	if err := envconfig.Process("", &action); err != nil {
		log.Fatal(err.Error())
	}
	log.SetPrefix(action.MyName + " ")
	log.Printf("using PORT=%d", action.Port)

	opts := make([]cehttp.Option, 0)
	//opts = append(opts, cloudevents.WithTarget(server.URL))
	//opts = append(opts, cloudevents.WithPort(0)) // random port
	opts = append(opts, cloudevents.WithPort(action.Port))
	opts = append(opts, cloudevents.WithPath("/"))
	opts = append(opts, cloudevents.WithGetHandlerFunc(action.handleGet))

	p, err := cloudevents.NewHTTP(opts...)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	client, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	action.client = client
	log.Printf("starting forwarding to %s", action.Target)
	if action.Target == "print" {
		action.PrintEvent = true
	}
	if action.SleepSeconds > 0 {
		log.Printf("delay sleep set to %f seconds before each forwarding", action.SleepSeconds)
	}
	if err := client.StartReceiver(context.Background(), action.ReceiveAndReply); err != nil {
		log.Fatalf("Failed to start receiver: %s", err.Error())
	}
	log.Printf("stopped forwarding to %s", action.Target)
}
