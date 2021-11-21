package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/net/context/ctxhttp"
)

// Client is a GraphQL client.
type Client struct {
	// Strict will force the decoder to use only the `graphql` structural flag.
	// If you set this to false, then it will use `graphql` as the first-class structural flag to use.
	// If it is not available, it will attempt to use the `json` structural flag.
	//
	// Defaults to false.
	Strict     bool
	url        string // GraphQL server URL.
	httpClient *http.Client
	// Headers allows you additional headers when performing the graphql request.
	Headers map[string]string
}

// ManualRequest allows you to define the graphql request in string format, and specify the variable where to
// unmarshal the JSON result.
type ManualRequest struct {
	// The GraphQL Query or Mutation, in string format.
	Query string
	// The variables used in the GraphQL query or mutation.
	Variables map[string]interface{}
	// Result is where the JSON response of the request will be decoded.
	//
	// Make sure that this is an pointer type (address to the struct you wish to decode into).
	Result interface{}
}

// NewClient creates a GraphQL client targeting the specified GraphQL server URL.
// If httpClient is nil, then http.DefaultClient is used.
func NewClient(url string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		url:        url,
		httpClient: httpClient,
	}
}

// Query executes a single GraphQL query request,
// with a query derived from q, populating the response into it.
// q should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Query(ctx context.Context, request ManualRequest, variables map[string]interface{}) error {
	return c.Do(ctx, queryOperation, request, variables, "")
}

// Mutate executes a single GraphQL mutation request,
// with a mutation derived from m, populating the response into it.
// m should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Mutate(ctx context.Context, request ManualRequest, variables map[string]interface{}) error {
	return c.Do(ctx, mutationOperation, request, variables, "")
}

// DoRaw executes a single GraphQL operation.
// return raw message and error
//
// Deprecated: Currently deprecated; will revisit this later.
func (c *Client) DoRaw(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}, name string) (*json.RawMessage, error) {
	var query string

	var manualRequest *ManualRequest

	mr, ok := v.(ManualRequest)

	if ok {
		manualRequest = &mr
		query = manualRequest.Query

	} else {
		switch op {
		case queryOperation:
			query = constructQuery(v, variables, name)
		case mutationOperation:
			query = constructMutation(v, variables, name)
		}
	}

	in := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables,omitempty"`
	}{
		Query:     query,
		Variables: variables,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(in)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequest("POST", c.url, &buf)

	if err != nil {
		return nil, err
	}

	for key, value := range c.Headers {
		httpRequest.Header.Add(key, value)
	}

	resp, err := ctxhttp.Do(ctx, c.httpClient, httpRequest)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 OK status code: %v body: %q", resp.Status, body)
	}
	var out struct {
		Data   *json.RawMessage
		Errors errors
		//Extensions interface{} // Unused.
	}

	// If input was a manual request, then use output from manual request
	if manualRequest != nil {

		var target interface{} = v

		if manualRequest != nil {
			target = manualRequest.Result
		}

		err = json.NewDecoder(resp.Body).Decode(target)
		return nil, err
	}

	// Do standard
	err = json.NewDecoder(resp.Body).Decode(&out)

	if err != nil {
		// TODO: Consider including response body in returned error, if deemed helpful.
		return nil, err
	}

	if len(out.Errors) > 0 {
		return out.Data, out.Errors
	}

	return out.Data, nil
}

// Do executes a single GraphQL operation and unmarshal json.
func (c *Client) Do(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}, name string) error {

	var query string
	var manualRequest *ManualRequest

	mr, ok := v.(ManualRequest)

	if ok {
		manualRequest = &mr
		query = manualRequest.Query

	} else {
		switch op {
		case queryOperation:
			query = constructQuery(v, variables, name)
		case mutationOperation:
			query = constructMutation(v, variables, name)
		}
	}

	in := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables,omitempty"`
	}{
		Query:     query,
		Variables: variables,
	}
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(in)
	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequest("POST", c.url, &buf)

	if err != nil {
		return err
	}

	for key, value := range c.Headers {
		httpRequest.Header.Add(key, value)
	}

	resp, err := ctxhttp.Do(ctx, c.httpClient, httpRequest)

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("non-200 OK status code: %v body: %q", resp.Status, body)
	}
	var out struct {
		Data   *json.RawMessage
		Errors errors
		//Extensions interface{} // Unused.
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		// TODO: Consider including response body in returned error, if deemed helpful.
		return err
	}
	if out.Data != nil {

		var target interface{} = v

		if manualRequest != nil {
			target = manualRequest.Result
		}

		err := json.Unmarshal(*out.Data, target)
		if err != nil {
			// TODO: Consider including response body in returned error, if deemed helpful.
			return err
		}
	}
	if len(out.Errors) > 0 {
		return out.Errors
	}
	return nil
}

// errors represents the "errors" array in a response from a GraphQL server.
// If returned via error interface, the slice is expected to contain at least 1 element.
//
// Specification: https://facebook.github.io/graphql/#sec-Errors.
type errors []struct {
	Extensions interface{}
	Message    string
	Locations  []struct {
		Line   int
		Column int
	}
}

// Error implements error interface.
func (e errors) Error() string {
	b := strings.Builder{}
	for _, err := range e {
		b.WriteString(fmt.Sprintf("Message: %s, Locations: %+v", err.Message, err.Locations))
	}
	return b.String()
}

type operationType uint8

const (
	queryOperation operationType = iota
	mutationOperation
	//subscriptionOperation // Unused.
)
