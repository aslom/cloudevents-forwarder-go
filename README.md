# Forward Cloud Events to another HTTP endpoint

# Quick Start

Start any HTTP endpoint that can receive Cloud Events over HTTP.

For quick testing you can use this forwarder started without forwarding on HTTP port 8082:

```
export PORT=8002
export CLOUDEVENTS_FORWARDER_TARGET=print
```



```
export CLOUDEVENTS_FORWARDER_TARGET=http://localhost:8002
go run .
```

## Running as Kubernetes Deployments

To allow quick customization environment variables are used in YAML files:

Use existing cloud events receiver or for testing run Knative event-display

```

```


```
export CLOUDEVENTS_FORWARDER_TARGET=http://localhost:8002
kubectl apply -f 
```


## Running using Knative Serving (scale to zero)
