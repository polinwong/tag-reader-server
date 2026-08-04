#!/usr/bin/bash

echo "Building binrary program..."
flags="-X 'main.GoVersion=$(go version)' -X 'main.BuildTime=`date +"%Y-%m-%d %H:%M:%S"`' -X 'main.GitCommit=`git rev-parse HEAD`'"
if [ "$1" == "win" ]; then
    env GOOS=windows GOARCH=amd64 go build -ldflags "$flags" -o "sourceserver.exe"
    echo "Build win-amd64 golang binrary done! File is \"sourceserver.exe\""
else
    env GOOS=linux GOARCH=amd64 go build -ldflags "$flags" -o "sourceserver-linux"
    echo "Build linux-amd64 golang binrary done! File is \"sourceserver-linux\""
fi