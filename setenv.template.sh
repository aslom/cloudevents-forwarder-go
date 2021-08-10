export IMAGE_NAME=${DOCKER_REGISTRY:-docker.io/aslom}/cloudevents-forwarder-go:v0.1
echo IMAGE_NAME=$IMAGE_NAME
export CLOUDEVENTS_FORWARDER_TARGET=http://event-display
export CLOUDEVENTS_FORWARDER_SLEEP_SECONDS=0