#!/bin/bash
cat internal/server/nabuauth.go | grep -v 'fail := func' > tmp
