# aws-lambda-custom-runtime-kit

AWS Lambda Custom Runtime create kit.

[![test status](https://github.com/WinterYukky/aws-lambda-custom-runtime-kit/actions/workflows/test.yml/badge.svg?branch=main "test status")](https://github.com/WinterYukky/aws-lambda-custom-runtime-kit/actions)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)

## Install

```shell
go get github.com/WinterYukky/aws-lambda-custom-runtime-kit
```

## Get Started

Create struct that implemented `AWSLambdaRuntime`, and call `NewAWSLambdaCustomRuntime(runtime).Invoke()`.  
When result type is string, then string is output as is. Else marshal to json string.

### Example

#### Write Custom Runtime

`main.go`
```go
package main

import (
	"fmt"
	"log"
	"os/exec"

	crkit "github.com/WinterYukky/aws-lambda-custom-runtime-kit"
)

type BashRuntime struct {}

func (b BashRuntime) Setup(env *crkit.AWSLambdaRuntimeEnvironemnt) error {
	return nil
}

func (b BashRuntime) Invoke(event []byte, context *crkit.Context) (interface{}, error) {
	source := fmt.Sprintf("%v/%v.sh", env.LambdaTaskRoot, env.Handler)
	output, err := exec.Command("sh", source).Output()
	if err != nil {
		return nil, err
	}
	return string(output), nil
}

func (b BashRuntime) Cleanup(env *crkit.AWSLambdaRuntimeEnvironemnt) {}

func main() {
	bashRuntime := BashRuntime{}
	customRuntime := crkit.NewAWSLambdaCustomRuntime(bashRuntime)
	if err := customRuntime.Invoke(); err != nil {
		log.Fatalf("Failed to invoke lambda: %v", err)
	}
}
```

#### Build as bootstrap.

```shell
runtime-tutorial$ go build -a -tags netgo -installsuffix netgo --ldflags '-extldflags "-static"' -o bootstrap
runtime-tutorial$ zip runtime.zip bootstrap
```

You can upload runtime.zip and create Lambda layer.
After then, you can invoke the function!
