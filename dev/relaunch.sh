#!/bin/sh

echo 'Stopping cdrata...'
killall -w -INT cdrata
sleep 1

echo 'Rebuilding...'
cd $GOPATH/src/github.com/commanderu/cdrata

git diff --no-ext-diff --quiet --exit-code
if [ $? -ne 0 ]; then
  echo "Dirty git workspace. Bailing!"
  exit 1
fi

git checkout master
git pull --ff-only origin master
SHORTREV=$(git rev-parse --short HEAD)
go build -v -ldflags "-X main.CommitHash=${SHORTREV}"

echo 'Launching!'
./cdrata
