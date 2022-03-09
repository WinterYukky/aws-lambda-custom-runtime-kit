package awslambdacustomruntimekit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

// Context is AWS Lambda event.
type Context struct {
	// The request ID, which identifies the request that triggered the function invocation.
	//  // example
	//  "8476a536-e9f4-11e8-9739-2dfe598c3fcd"
	RequestID string
	// The date that the function times out in Unix time milliseconds.
	//  // example
	//  "1542409706888"
	DeadlineMs string
	// The ARN of the Lambda function, version, or alias that's specified in the invocation.
	//  // example
	//  "arn:aws:lambda:us-east-2:123456789012:function:custom-runtime"
	InvokedFunctionArn string
	// AWS X-Ray tracing header.
	//  // example
	//  "Root=1-5bef4de7-ad49b0e87f6ef6c87fc2e700;Parent=9a9197af755a6419;Sampled=1"
	TraceID string
	// For invocations from the AWS Mobile SDK, data about the client application and device.
	ClientContext string
	// For invocations from the AWS Mobile SDK, data about the Amazon Cognito identity provider.
	CognitoIdentity string
	AWSLambdaRuntimeEnvironemnt
}

// AWSLambdaRuntimeEnvironemnt is needy environment info for to invoke handler
type AWSLambdaRuntimeEnvironemnt struct {
	AWSLambdaRuntimeAPI string
	LambdaTaskRoot      string
	Handler             string
}

// AWSLambdaRuntimeError is error of AWS Lambda runtime error.
type AWSLambdaRuntimeError struct {
	ErrorMessage string   `json:"errorMessage"`
	ErrorType    string   `json:"errorType"`
	StackTrace   []string `json:"stackTrace"`
}

func (a AWSLambdaRuntimeError) Error() string {
	return a.ErrorMessage
}

// AWSLambdaRuntime is interface of any runtime.
type AWSLambdaRuntime interface {
	// Setup before invoke the function handler.
	Setup(env *AWSLambdaRuntimeEnvironemnt) error
	// Invoke the function handler and return the result.
	Invoke(event []byte, context *Context) (interface{}, error)
	// Cleanup after invoke the function handler.
	Cleanup(env *AWSLambdaRuntimeEnvironemnt)
}

// HTTPClient is dependency of AWSLambdaCustomRuntime.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// AWS Lambda Custom Runtime
type AWSLambdaCustomRuntime struct {
	runtime             AWSLambdaRuntime
	httpClient          HTTPClient
	awsLambdaRuntimeAPI string
	lambdaTaskRoot      string
	handlerName         string
	infinity            bool
}

// Initialize AWSLambdaCustomRuntime
func NewAWSLambdaCustomRuntime(runtime AWSLambdaRuntime) *AWSLambdaCustomRuntime {
	return &AWSLambdaCustomRuntime{
		runtime:             runtime,
		httpClient:          http.DefaultClient,
		awsLambdaRuntimeAPI: os.Getenv("AWS_LAMBDA_RUNTIME_API"),
		lambdaTaskRoot:      os.Getenv("LAMBDA_TASK_ROOT"),
		handlerName:         os.Getenv("_HANDLER"),
		infinity:            true,
	}
}

// Invoke the runtime handler
func (a *AWSLambdaCustomRuntime) Invoke() error {
	env := &AWSLambdaRuntimeEnvironemnt{
		AWSLambdaRuntimeAPI: a.awsLambdaRuntimeAPI,
		LambdaTaskRoot:      a.lambdaTaskRoot,
		Handler:             a.handlerName,
	}
	if err := a.runtime.Setup(env); err != nil {
		return a.initializeError(err)
	}
	for {
		event, header, err := a.getEvent()
		if err != nil {
			return a.initializeError(AWSLambdaRuntimeError{
				ErrorMessage: err.Error(),
				ErrorType:    "Initialize.GetEvent",
			})
		}
		a.propagateTracingHeader(header)
		context := a.newContext(env, header)

		result, err := a.runtime.Invoke(event, context)
		if err != nil {
			return a.invokeError(context, err)
		}
		if err = a.handleResponse(context, result); err != nil {
			return a.invokeError(context, err)
		}

		if !a.infinity {
			break
		}
	}
	a.runtime.Cleanup(env)
	return nil
}

func (a *AWSLambdaCustomRuntime) getEvent() ([]byte, http.Header, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%v/2018-06-01/runtime/invocation/next", a.awsLambdaRuntimeAPI), nil)
	res, err := a.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get event data: %v", err)
	}
	event, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read event data: %v", err)
	}
	return event, res.Header, nil
}

func (a *AWSLambdaCustomRuntime) propagateTracingHeader(header http.Header) {
	os.Setenv("_X_AMZN_TRACE_ID", header.Get("Lambda-Runtime-Trace-Id"))
}

func (a *AWSLambdaCustomRuntime) newContext(env *AWSLambdaRuntimeEnvironemnt, header http.Header) *Context {
	return &Context{
		RequestID:                   header.Get("Lambda-Runtime-Aws-Request-Id"),
		DeadlineMs:                  header.Get("Lambda-Runtime-Deadline-Ms"),
		InvokedFunctionArn:          header.Get("Lambda-Runtime-Invoked-Function-Arn"),
		TraceID:                     header.Get("Lambda-Runtime-Trace-Id"),
		ClientContext:               header.Get("Lambda-Runtime-Client-Context"),
		CognitoIdentity:             header.Get("Lambda-Runtime-Cognito-Identity"),
		AWSLambdaRuntimeEnvironemnt: *env,
	}
}

func (a *AWSLambdaCustomRuntime) handleResponse(event *Context, value interface{}) error {
	var body []byte
	if v, ok := value.(string); ok {
		body = []byte(v)
	} else {
		body, _ = json.Marshal(value)
	}
	url := fmt.Sprintf("http://%v/2018-06-01/runtime/invocation/%v/response", a.awsLambdaRuntimeAPI, event.RequestID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	_, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send response data: %v", err)
	}
	return nil
}

func newAWSLambdaRuntimeError(err error) AWSLambdaRuntimeError {
	if e, ok := err.(AWSLambdaRuntimeError); ok {
		return e
	}
	return AWSLambdaRuntimeError{
		ErrorMessage: err.Error(),
		ErrorType:    "Extension.UnknownReason",
	}
}

func (a *AWSLambdaCustomRuntime) errorResponse(url string, e error) error {
	runtimeError := newAWSLambdaRuntimeError(e)
	body, _ := json.Marshal(runtimeError)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header["Lambda-Runtime-Function-Error-Type"] = []string{runtimeError.ErrorType}
	_, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send response data: %v", err)
	}
	return e
}

func (a *AWSLambdaCustomRuntime) initializeError(err error) error {
	url := fmt.Sprintf("http://%v/2018-06-01/runtime/init/error", a.awsLambdaRuntimeAPI)
	return a.errorResponse(url, err)
}

func (a *AWSLambdaCustomRuntime) invokeError(event *Context, err error) error {
	url := fmt.Sprintf("http://%v/2018-06-01/runtime/invocation/%v/error", a.awsLambdaRuntimeAPI, event.RequestID)
	return a.errorResponse(url, err)
}
