package main

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestReadConfig_blank(t *testing.T) {
	os.Setenv("PROVIDERS", "")
	config, err := readConfig()
	want := &Response{
		StatusCode:      500,
		IsBase64Encoded: false,
		Body:            "{\"error\":\"ENVVAR PROVIDERS is invalid yaml\"}",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	if config != nil {
		t.Errorf("Config should be nil")
	}
	if !reflect.DeepEqual(err, want) {
		t.Errorf("unmarshalRequest() got = %v, want %v", err, want)
	}
}

func TestReadConfig_unset(t *testing.T) {
	os.Unsetenv("PROVIDERS")
	config, err := readConfig()
	want := &Response{
		StatusCode:      500,
		IsBase64Encoded: false,
		Body:            "{\"error\":\"ENVVAR PROVIDERS is required\"}",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	if config != nil {
		t.Errorf("Config should be nil")
	}
	if !reflect.DeepEqual(err, want) {
		t.Errorf("unmarshalRequest() got = %v, want %v", err, want)
	}
}

func TestReadConfig_invalid(t *testing.T) {
	os.Setenv("PROVIDERS", "\"sss\"sss\"")
	config, err := readConfig()
	want := &Response{
		StatusCode:      500,
		IsBase64Encoded: false,
		Body:            "{\"error\":\"ENVVAR PROVIDERS is invalid yaml\"}",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	if config != nil {
		t.Errorf("Config should be nil")
	}
	if !reflect.DeepEqual(err, want) {
		t.Errorf("unmarshalRequest() got = %v, want %v", err, want)
	}
}

func Test_unmarshalRequest(t *testing.T) {
	type args struct {
		request Request
	}
	accountNumber := "12345678"
	providers := []string{"provider1", "provider2"}
	tests := []struct {
		name  string
		args  args
		want  *BankAccountValidationRequest
		want1 *Response
	}{
		{name: "valid",
			args: args{
				request: Request{
					Body: "{\"accountNumber\": \"12345678\"}",
				},
			},
			want: &BankAccountValidationRequest{
				AccountNumber: &accountNumber,
			},
			want1: nil,
		},
		{name: "validWithProvider",
			args: args{
				request: Request{
					Body: "{\"accountNumber\": \"12345678\", \"providers\": [\"provider1\", \"provider2\"]}",
				},
			},
			want: &BankAccountValidationRequest{
				AccountNumber: &accountNumber,
				Providers:     &providers,
			},
			want1: nil,
		},
		{name: "missingAccount",
			args: args{
				request: Request{
					Body: "{\"david\": \"12345678\"}",
				},
			},
			want: nil,
			want1: &Response{
				StatusCode:      500,
				IsBase64Encoded: false,
				Body:            "{\"error\":\"account number missing from payload\"}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
		{name: "invalidJson",
			args: args{
				request: Request{
					Body: "{\"accountNumber: \"12345678\"}",
				},
			},
			want: nil,
			want1: &Response{
				StatusCode:      500,
				IsBase64Encoded: false,
				Body:            "{\"error\":\"invalid json payload\"}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := unmarshalRequest(tt.args.request)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unmarshalRequest() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("unmarshalRequest() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_handleError(t *testing.T) {
	type args struct {
		err     error
		message string
	}
	tests := []struct {
		name string
		args args
		want *Response
	}{
		{name: "error",
			args: args{
				err:     errors.New("Error"),
				message: "error",
			},
			want: &Response{
				StatusCode:      500,
				IsBase64Encoded: false,
				Body:            "{\"error\":\"error\"}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
		{name: "error2",
			args: args{
				err:     nil,
				message: "error",
			},
			want: &Response{
				StatusCode:      500,
				IsBase64Encoded: false,
				Body:            "{\"error\":\"error\"}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
		{name: "error3",
			args: args{
				err:     errors.New("Error"),
				message: "",
			},
			want: &Response{
				StatusCode:      500,
				IsBase64Encoded: false,
				Body:            "{\"error\":\"\"}",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handleError(tt.args.err, tt.args.message); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("handleError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_providersToCall(t *testing.T) {
	type args struct {
		providers []Provider
		filter    *[]string
	}
	filter1 := []string{"provider1"}
	filter2 := []string{"provider3"}
	filter3 := []string{"provider1", "provider3"}
	filter4 := []string{}
	tests := []struct {
		name string
		args args
		want []Provider
	}{
		{name: "nofilter",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				filter: nil,
			},
			want: []Provider{
				{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
				{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
			},
		},
		{name: "filter1",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				filter: &filter1,
			},
			want: []Provider{
				{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
			},
		},
		{name: "filter2",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				filter: &filter2,
			},
			want: []Provider{},
		},
		{name: "filter3",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				filter: &filter3,
			},
			want: []Provider{
				{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
			},
		},
		{name: "filter4",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				filter: &filter4,
			},
			want: []Provider{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providersToCall(tt.args.providers, tt.args.filter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("providersToCall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkProviders(t *testing.T) {
	type args struct {
		accountNumber string
		providers     []Provider
	}
	tests := []struct {
		name string
		args args
		want BankAccountValidationResponse
	}{
		{name: "filter4",
			args: args{
				providers: []Provider{
					{Name: "provider1", URL: "https://provider1.com/v1/api/account/validate"},
					{Name: "provider2", URL: "https://provider2.com/v1/api/account/validate"},
				},
				accountNumber: "12345678",
			},
			want: BankAccountValidationResponse{
				Result: []BankAccountValidationResult{
					{Provider: "provider2", IsValid: false},
					{Provider: "provider1", IsValid: false},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkProviders(tt.args.accountNumber, tt.args.providers); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("checkProviders() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TODO implement http mocks (although likely to do this as an E2E test)
