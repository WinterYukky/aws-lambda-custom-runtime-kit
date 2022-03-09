package awslambdacustomruntimekit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

type MockHTTPClient struct {
	args []*http.Request
	do   func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.do(req)
}

var httpClientCreateCount = 0

type MockHTTPClientProps struct {
	GetEvent        func(req *http.Request) (*http.Response, error)
	InitializeError func(req *http.Request) (*http.Response, error)
	InvokeError     func(req *http.Request) (*http.Response, error)
	Response        func(req *http.Request) (*http.Response, error)
}

func NewMockHTTPClient(props MockHTTPClientProps) *MockHTTPClient {
	client := &MockHTTPClient{}
	client.do = func(req *http.Request) (*http.Response, error) {
		httpClientCreateCount++
		client.args = append(client.args, req)
		switch {
		case strings.HasSuffix(req.URL.Path, "/2018-06-01/runtime/invocation/next"):
			return props.GetEvent(req)
		case strings.HasSuffix(req.URL.Path, "/runtime/init/error"):
			return props.InitializeError(req)
		case strings.HasSuffix(req.URL.Path, "/error"):
			return props.InvokeError(req)
		default:
			return props.Response(req)
		}
	}
	return client
}

type AlwaysFailReader struct{}

func (a AlwaysFailReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("always return error")
}

type InstantRuntime struct {
	setup  func(env *AWSLambdaRuntimeEnvironemnt) error
	invoke func(event []byte, context *Context) (interface{}, error)
}

func (i *InstantRuntime) Setup(env *AWSLambdaRuntimeEnvironemnt) error {
	return i.setup(env)
}
func (i *InstantRuntime) Invoke(event []byte, context *Context) (interface{}, error) {
	return i.invoke(event, context)
}
func (i *InstantRuntime) Cleanup(env *AWSLambdaRuntimeEnvironemnt) {}

func NewBody(body string) io.ReadCloser {
	return io.NopCloser(bytes.NewReader([]byte(body)))
}

func TestAWSLambdaCustomRuntime_Invoke(t *testing.T) {
	getResponse := func(lambda *AWSLambdaCustomRuntime) string {
		args := lambda.httpClient.(*MockHTTPClient).args
		if len(args) == 0 {
			return ""
		}
		lastReq := args[len(args)-1]
		if !strings.HasSuffix(lastReq.URL.Path, "/response") {
			return ""
		}
		body, _ := ioutil.ReadAll(lastReq.Body)
		return string(body)
	}

	type fields struct {
		runtime             AWSLambdaRuntime
		httpClient          HTTPClient
		awsLambdaRuntimeAPI string
		lambdaTaskRoot      string
		handler             string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{
			name: "When failed to setup then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					InitializeError: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error {
						return errors.New("Intentional error")
					},
					invoke: func(event []byte, context *Context) (interface{}, error) { return "", nil },
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "When failed to setup and failed to response error then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					InitializeError: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("Intentional error")
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error {
						return errors.New("Intentional error")
					},
					invoke: func(event []byte, context *Context) (interface{}, error) { return "", nil },
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "When failed to get event response then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, errors.New("Intentional error")
					},
					InitializeError: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup:  func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) { return "", nil },
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "When failed to read event body then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Body: io.NopCloser(AlwaysFailReader{}),
						}, nil
					},
					InitializeError: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup:  func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) { return "", nil },
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "When failed to invoke the handler then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Body: NewBody(`{"key1":"value1"}`),
						}, nil
					},
					InvokeError: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) {
						return nil, errors.New("Intentional error")
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "When failed to response the handler result then return error",
			fields: fields{
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Body: NewBody(`{"key1":"value1"}`),
						}, nil
					},
					Response: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("Intentional error")
					},
					InvokeError: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) {
						return map[string]string{
							"name": "WinterYukky",
						}, nil
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Get event URL is should http://$AWS_LAMBDA_RUNTIME_API/2018-06-01/runtime/invocation/next",
			fields: fields{
				awsLambdaRuntimeAPI: "unit-test-runtime-api",
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						if req.URL.String() != "http://unit-test-runtime-api/2018-06-01/runtime/invocation/next" {
							return nil, fmt.Errorf("get event URL is should http://$AWS_LAMBDA_RUNTIME_API/2018-06-01/runtime/invocation/next, got %v", req.URL.String())
						}
						return &http.Response{
							Body: NewBody(`{"key1":"value1"}`),
						}, nil
					},
					Response: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) {
						return map[string]string{
							"name": "WinterYukky",
						}, nil
					},
				},
			},
			want:    `{"name":"WinterYukky"}`,
			wantErr: false,
		},
		{
			name: "String result is output as is",
			fields: fields{
				awsLambdaRuntimeAPI: "unit-test-runtime-api",
				httpClient: NewMockHTTPClient(MockHTTPClientProps{
					GetEvent: func(req *http.Request) (*http.Response, error) {
						if req.URL.String() != "http://unit-test-runtime-api/2018-06-01/runtime/invocation/next" {
							return nil, fmt.Errorf("get event URL is should http://$AWS_LAMBDA_RUNTIME_API/2018-06-01/runtime/invocation/next, got %v", req.URL.String())
						}
						return &http.Response{
							Body: NewBody(`{"key1":"value1"}`),
						}, nil
					},
					Response: func(req *http.Request) (*http.Response, error) {
						return &http.Response{}, nil
					},
				}),
				runtime: &InstantRuntime{
					setup: func(env *AWSLambdaRuntimeEnvironemnt) error { return nil },
					invoke: func(event []byte, context *Context) (interface{}, error) {
						return `{"name":"WinterYukky"}`, nil
					},
				},
			},
			want:    `{"name":"WinterYukky"}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lambda := &AWSLambdaCustomRuntime{
				runtime:             tt.fields.runtime,
				httpClient:          tt.fields.httpClient,
				awsLambdaRuntimeAPI: tt.fields.awsLambdaRuntimeAPI,
				lambdaTaskRoot:      tt.fields.lambdaTaskRoot,
				handlerName:         tt.fields.handler,
			}
			err := lambda.Invoke()
			if (err != nil) != tt.wantErr {
				t.Errorf("AWSLambdaCustomRuntime.Invoke() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			got := getResponse(lambda)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AWSLambdaCustomRuntime.Invoke() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewAWSLambdaCustomRuntime(t *testing.T) {
	type args struct {
		runtime AWSLambdaRuntime
	}
	tests := []struct {
		name  string
		args  args
		setup func()
		want  *AWSLambdaCustomRuntime
	}{
		{
			name: "Setup AWSLambdaCustomRuntime",
			args: args{
				runtime: &InstantRuntime{},
			},
			setup: func() {
				os.Setenv("AWS_LAMBDA_RUNTIME_API", "unit-test-runtime-api")
				os.Setenv("LAMBDA_TASK_ROOT", "unit-test-task-root")
				os.Setenv("_HANDLER", "unit-test-handler")
			},
			want: &AWSLambdaCustomRuntime{
				runtime:             &InstantRuntime{},
				httpClient:          http.DefaultClient,
				awsLambdaRuntimeAPI: "unit-test-runtime-api",
				lambdaTaskRoot:      "unit-test-task-root",
				handlerName:         "unit-test-handler",
				infinity:            true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			if got := NewAWSLambdaCustomRuntime(tt.args.runtime); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAWSLambdaCustomRuntime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAWSLambdaRuntimeError_Error(t *testing.T) {
	type fields struct {
		ErrorMessage string
		ErrorType    string
		StackTrace   []string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "Should return ErrorMessage",
			fields: fields{
				ErrorMessage: "unit test",
				ErrorType:    "Unit.Test",
				StackTrace:   []string{},
			},
			want: "unit test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := AWSLambdaRuntimeError{
				ErrorMessage: tt.fields.ErrorMessage,
				ErrorType:    tt.fields.ErrorType,
				StackTrace:   tt.fields.StackTrace,
			}
			if got := a.Error(); got != tt.want {
				t.Errorf("AWSLambdaRuntimeError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
