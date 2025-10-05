#!/bin/bash

BUILD_DIR="../api-auth-verifier-lambda"
OUTPUT="bootstrap"

# Build with explicit output path
cd "$BUILD_DIR"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$OUTPUT" ./cmd/api-auth-verifier

# Move back and create zip
cd -
mv "$BUILD_DIR/$OUTPUT" ./"$OUTPUT"
zip lambda-handler.zip "$OUTPUT"
rm "$OUTPUT"

echo "✓ Lambda package created: lambda-handler.zip"
