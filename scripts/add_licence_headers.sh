#!/bin/bash

find . -path ./vendor -prune -o -type f -name "*.go" -print | while read file; do
  if ! grep -q Copyright $file; then
    cat scripts/resources/licence_header.txt $file > $file.new && mv $file.new $file
  fi
done
