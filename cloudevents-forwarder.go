package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/kelseyhightower/envconfig"
)

type ForwardEventsStats struct {
}

type ForwardingAction struct {
	client        cloudevents.Client
	httpTransport *http.Transport

	MyName string `envconfig:"NAME" default:"CloudEventsForwarder"`

	SleepSeconds float64 `envconfig:"CLOUDEVENTS_FORWARDER_SLEEP_SECONDS" default:"0"`

	// If the K_SINK environment variable is set, then events are sent there,
	// otherwise we simply reply to the inbound request.
	Target string `envconfig:"CLOUDEVENTS_FORWARDER_TARGET" required:"true"`

	// Port on which to listen for cloudevents
	Port int `envconfig:"PORT" default:"8080"`

	//LogLevel int `envconfig:"LOG_LEVEL" default:"info"`

	PrintEvent bool `envconfig:"PRINT_EVENT" default:"false"`

	SkipSdk bool `envconfig:"SKIP_SDK" default:"false"`
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

func runWithCloudEventsSdk(action *ForwardingAction) {
	log.Printf("starting forwarding using SDK to %s", action.Target)

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
	if err := client.StartReceiver(context.Background(), action.ReceiveAndReply); err != nil {
		log.Fatalf("Failed to start receiver: %s", err.Error())
	}
	log.Printf("stopped forwarding to %s", action.Target)
}

func (action *ForwardingAction) requestHandler(res http.ResponseWriter, req *http.Request) {
	contentType := req.Header.Get("Content-type")
	//fmt.Println(contentType)
	jsonMap := make(map[string]interface{})
	body, err := ioutil.ReadAll(req.Body)
	defer req.Body.Close()
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(res, "can't read body", http.StatusBadRequest)
		return
	}
	if strings.Contains(contentType, "application/cloudevents+json") {
		err = json.Unmarshal(body, &jsonMap)
		if err != nil {
			log.Printf("Error unmarshaling body to JSON: %v", err)
			http.Error(res, err.Error(), 500)
			return
		}
	} else if strings.Contains(contentType, "application/json") {
		for name, values := range req.Header {
			for _, value := range values {
				//fmt.Println("header", name, value)
				if strings.HasPrefix(name, "Ce-") {
					key := strings.ToLower(strings.TrimPrefix(name, "Ce-"))
					//fmt.Println("json", key, value)
					jsonMap[key] = value
				}
			}
		}
		jsonData := make(map[string]interface{})
		err = json.Unmarshal(body, &jsonData)
		if err != nil {
			log.Printf("Error unmarshaling body to JSON: %v", err)
			http.Error(res, err.Error(), 500)
			return
		}
		jsonMap["data"] = jsonData
	} else {
		msg := "Content-Type header " + contentType + " is not supported"
		http.Error(res, msg, http.StatusUnsupportedMediaType)
		return
	}

	if action.PrintEvent {
		log.Printf("event %v", jsonMap)
	}
	if action.Target == "print" {
		return
	}

	// forward cloud event JSON to HTTP
	event, err := json.Marshal(jsonMap)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sending to %s event %s\n", action.Target, event)
	sendReq, sendingErr := http.NewRequest("POST", action.Target, bytes.NewBuffer(event))
	if sendingErr != nil {
		log.Println(err)
	}
	sendReq.Header.Set("Content-Type", "application/cloudevents+json; charset=UTF-8")
	resp, err := action.httpTransport.RoundTrip(sendReq)
	// TODO check response HTTP code?
	// count successes and errors
	if err != nil {
		log.Printf("Failed to forward event: %v", err)
		http.Error(res, err.Error(), 500)
		return
	}

	//log.Printf("Received JSON %s", jsonMap)
	//data := []byte("{'Hello' :  'World!'}")

	log.Printf("sleeping %f seconds after forwarding event from source %s id %s with result %v", action.SleepSeconds, jsonMap["source"], jsonMap["id"], resp)
	if action.SleepSeconds > 0 {
		milliseconds := action.SleepSeconds * 1000
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	}

	//res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	//res.Write(data)

}

func runWithHttp(action *ForwardingAction) {
	action.httpTransport = &http.Transport{}
	http.HandleFunc("/", action.requestHandler)
	loc := ":" + strconv.Itoa(action.Port)
	fmt.Printf("starting forwarding HTTP server on %s\n", loc)
	if err := http.ListenAndServe(loc, nil); err != nil {
		log.Fatalf("error in ListenAndServe: %s", err)
	}
}

func main() {
	action := ForwardingAction{}
	if err := envconfig.Process("", &action); err != nil {
		log.Fatal(err.Error())
	}
	log.SetPrefix(action.MyName + " ")
	log.Printf("using PORT=%d", action.Port)
	if action.Target == "print" {
		action.PrintEvent = true
	}
	if action.SleepSeconds > 0 {
		log.Printf("delay sleep set to %f seconds before each forwarding", action.SleepSeconds)
	}

	if action.SkipSdk {
		runWithHttp(&action)
	} else {
		runWithCloudEventsSdk(&action)
	}
}
