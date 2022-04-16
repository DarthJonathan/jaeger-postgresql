#/bin/bash

set -e

VERSION=v$(cat version.json | jq -r '.version')

REPO="asia-southeast1-docker.pkg.dev/flowscan/images/utils/jaeger-postgresql"
FULL_IMAGE_TAG="$REPO:$VERSION"

# Check for existing image
# RESULT=$(gcloud artifacts docker images list $REPO --include-tags --filter "tags:$VERSION" --format json 2> /dev/null)
# if [ -n "$RESULT" ] && (( $(echo $RESULT | jq length) > 0 )); then
#   echo "$FULL_IMAGE_TAG already exists. Skipping!"
#   exit
# fi

# Build the docker image
echo "Building docker image for $FULL_IMAGE_TAG"
docker build --tag ${FULL_IMAGE_TAG} .
docker push $FULL_IMAGE_TAG
