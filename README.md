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

#### Create a layer

```shell
runtime-tutorial$ aws lambda publish-layer-version --layer-name bash-runtime --zip-file fileb://runtime.zip
 {
    "Content": {
        "Location": "https://awslambda-us-west-2-layers.s3.us-west-2.amazonaws.com/snapshots/123456789012/bash-runtime-018c209b...",
        "CodeSha256": "bXVLhHi+D3H1QbDARUVPrDwlC7bssPxySQqt1QZqusE=",
        "CodeSize": 584,
        "UncompressedCodeSize": 0
    },
    "LayerArn": "arn:aws:lambda:us-west-2:123456789012:layer:bash-runtime",
    "LayerVersionArn": "arn:aws:lambda:us-west-2:123456789012:layer:bash-runtime:1",
    "Description": "",
    "CreatedDate": "2018-11-28T07:49:14.476+0000",
    "Version": 1
}
```

#### Update the function

```shell
runtime-tutorial$ aws lambda update-function-configuration --function-name bash-runtime \
--layers arn:aws:lambda:us-west-2:123456789012:layer:bash-runtime:1
{
    "FunctionName": "bash-runtime",
    "Layers": [
        {
            "Arn": "arn:aws:lambda:us-west-2:123456789012:layer:bash-runtime:1",
            "CodeSize": 584,
            "UncompressedCodeSize": 679
        }
    ]
    ...
}
```

`index.sh`
```sh
echo '{"key":"value"}'
```

```shell
runtime-tutorial$ zip function-only.zip index.sh
  adding: index.sh (deflated 24%)
runtime-tutorial$ aws lambda update-function-code --function-name bash-runtime --zip-file fileb://function-only.zip
{
    "FunctionName": "bash-runtime",
    "CodeSize": 270,
    "Layers": [
        {
            "Arn": "arn:aws:lambda:us-west-2:123456789012:layer:bash-runtime:7",
            "CodeSize": 584,
            "UncompressedCodeSize": 679
        }
    ]
    ...
}
```

After then, you can invoke the function!