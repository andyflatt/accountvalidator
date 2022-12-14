package main

/*
  Lambda function which can be exposed by AWS API Gateway.  It has the following requirements:

	1. Rest api, all messages in json - It requested a RESTful API but the example messages are very RPC.
	   I went for the RPC style in the example messages to match spec.
	2. Spring boot app - We discussed on the call that you would prefer Golang.  I can do this in spring boot if needed.
	3. Sufficient tests to demonstrate the app is working correctly.
	4. Data providers' url are set as properties and must not be stored in code. Demonstrate how the urls can be set for
	   production and non-production environments.  This is handled by the serverless framework. In production I would use
		 SSM Parameter store to store the configuration.
	5. The rest api should return response within 2 seconds. It is guaranteed that all external data providers will return
     data within 1 second.  There is threading, but depending on infrastructure depends on how may providers we could call
		to meet this SLA.  I did no performance tests.
*/
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	yaml "gopkg.in/yaml.v2"
)

/*
  TYPES
*/

type Config struct {
	Providers []Provider
}

type Provider struct {
	Name string
	URL  string
}

type BankAccountValidationRequest struct {
	AccountNumber *string   `json:"accountNumber"`
	Providers     *[]string `json:"providers"`
}

type BankAccountValidationResult struct {
	Provider string `json:"provider"`
	IsValid  bool   `json:"isValid"`
}

type BankAccountValidationResponse struct {
	Result []BankAccountValidationResult `json:"result"`
}

type DataProviderRequest struct {
	AccountNumber string `json:"accountNumber"`
}

type DataProviderResponse struct {
	IsValid bool `json:"isValid"`
}

// Response is of type APIGatewayProxyResponse as we are using the AWS Lambda Proxy Request functionality
type Response events.APIGatewayProxyResponse
type Request events.APIGatewayProxyRequest

// Handler is our lambda handler invoked by the `lambda.Start` function call
func (config *Config) Handler(ctx context.Context, request Request) (Response, error) {
	var buf bytes.Buffer

	// Get and validate the request
	validationRequest, errorResponse := unmarshalRequest(request)
	if errorResponse != nil {
		return *errorResponse, nil
	}

	// Create the response
	var response BankAccountValidationResponse = checkProviders(
		*validationRequest.AccountNumber,
		providersToCall(config.Providers, validationRequest.Providers))

	// Send the response
	body, err := json.Marshal(response)
	if err != nil {
		return Response{StatusCode: 404}, err
	}
	json.HTMLEscape(&buf, body)
	resp := Response{
		StatusCode:      200,
		IsBase64Encoded: false,
		Body:            buf.String(),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
	return resp, nil
}

func providersToCall(providers []Provider, filter *[]string) []Provider {
	if filter == nil {
		return providers
	}
	// Could do this once instead of on every request
	confMap := map[string]Provider{}
	for _, value := range providers {
		confMap[value.Name] = value
	}
	filteredProviders := []Provider{}
	for _, providerName := range *filter {
		providerConfig, exists := confMap[providerName]
		if exists {
			filteredProviders = append(filteredProviders, providerConfig)
		}
	}
	return filteredProviders
}

// Deserialises and validate request
func unmarshalRequest(request Request) (*BankAccountValidationRequest, *Response) {
	var validationRequest *BankAccountValidationRequest

	if err := json.Unmarshal([]byte(request.Body), &validationRequest); err != nil {
		return nil, handleError(err, "invalid json payload")
	}

	if validationRequest.AccountNumber == nil {
		message := "account number missing from payload"
		return nil, handleError(errors.New(message), message)
	}

	return validationRequest, nil
}

// Fire off sync calls to the providers
func checkProviders(accountNumber string, providers []Provider) BankAccountValidationResponse {
	channel := make(chan BankAccountValidationResult)
	var wg sync.WaitGroup

	for _, provider := range providers {
		wg.Add(1)
		go checkProvider(accountNumber, provider, channel, &wg)
	}

	// little bit lazy to have this annomymous and call itself.
	// It just waits for work to complete and close the channel.
	go func() {
		wg.Wait()
		close(channel)
	}()

	// An endless loop that just waits for results to come in through the channel
	// I am almost sure there is a nicer way to do this syntatically, but time is
	// short
	results := []BankAccountValidationResult{}
	for result := range channel {
		results = append(results, result)
	}
	return BankAccountValidationResponse{Result: results}
}

// Function to check a provider.
func checkProvider(accountNumber string, provider Provider, c chan BankAccountValidationResult, wg *sync.WaitGroup) {
	defer (*wg).Done()
	defaultResponse := BankAccountValidationResult{
		IsValid:  false,
		Provider: provider.Name,
	}
	client := http.Client{
    Timeout: 1 * time.Second,
	}

	// Make the http call
	values := map[string]string{"accountNumber": accountNumber}
	json_data, err := json.Marshal(values)
	if err != nil {
		log.Print(err)
		c <- defaultResponse
		return
	}

	response, err := client.Post(provider.URL, "application/json", bytes.NewBuffer(json_data)) // TODO POST with the right payload
	if err != nil {
		log.Print(err)
		c <- defaultResponse
		return
	}

	// Parse the response
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Print(err)
		c <- defaultResponse
		return
	}

	// Parse the json into a struct
	var providerResponse *DataProviderResponse
	if err := json.Unmarshal(bodyBytes, &providerResponse); err != nil {
		log.Print(err)
		c <- defaultResponse
		return
	}

	// Send the result to the channel
	c <- BankAccountValidationResult{
		IsValid:  providerResponse.IsValid,
		Provider: provider.Name,
	}
}

// Generic error handling response builder
func handleError(err error, message string) *Response {
	log.Print(err)
	var buf bytes.Buffer
	body, err := json.Marshal(map[string]interface{}{
		"error": message,
	})
	if err != nil {
		log.Print("Unable to serialise error message")
		log.Print(err)
	}
	json.HTMLEscape(&buf, body)
	return &Response{
		StatusCode:      500,
		IsBase64Encoded: false,
		Body:            buf.String(),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
}

// Lambda screwyness to make sure if the init fails it can still respond with an error
func (err Response) OnlyErrors() Response {
	return err
}

// Read the config from an ENVVAR
func readConfig() (*Config, *Response) {
	var providerYaml, exists = os.LookupEnv("PROVIDERS")
	if !exists {
		return nil, handleError(nil, "ENVVAR PROVIDERS is required")
	}
	var config *Config
	err := yaml.Unmarshal([]byte(providerYaml), &config)
	if err != nil || config == nil {
		return nil, handleError(nil, "ENVVAR PROVIDERS is invalid yaml")
	}
	return config, nil
}

func main() {
	config, err := readConfig()
	if err != nil {
		lambda.Start(err.OnlyErrors)
	} else {
		log.Println(config)
		lambda.Start(config.Handler)
	}
}
