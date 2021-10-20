package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/darrensapalo/go-graphql-client/internal/jsonutil"
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
	Query     string
	Variables map[string]interface{}
	Result    interface{}
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
func (c *Client) Query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	return c.do(ctx, queryOperation, q, variables, "")
}

// NamedQuery executes a single GraphQL query request, with operation name
func (c *Client) NamedQuery(ctx context.Context, name string, q interface{}, variables map[string]interface{}) error {
	return c.do(ctx, queryOperation, q, variables, name)
}

// Mutate executes a single GraphQL mutation request,
// with a mutation derived from m, populating the response into it.
// m should be a pointer to struct that corresponds to the GraphQL schema.
func (c *Client) Mutate(ctx context.Context, m interface{}, variables map[string]interface{}) error {
	return c.do(ctx, mutationOperation, m, variables, "")
}

// NamedMutate executes a single GraphQL mutation request, with operation name
func (c *Client) NamedMutate(ctx context.Context, name string, m interface{}, variables map[string]interface{}) error {
	return c.do(ctx, mutationOperation, m, variables, name)
}

// Query executes a single GraphQL query request,
// with a query derived from q, populating the response into it.
// q should be a pointer to struct that corresponds to the GraphQL schema.
// return raw bytes message.
func (c *Client) QueryRaw(ctx context.Context, q interface{}, variables map[string]interface{}) (*json.RawMessage, error) {
	return c.doRaw(ctx, queryOperation, q, variables, "")
}

// NamedQueryRaw executes a single GraphQL query request, with operation name
// return raw bytes message.
func (c *Client) NamedQueryRaw(ctx context.Context, name string, q interface{}, variables map[string]interface{}) (*json.RawMessage, error) {
	return c.doRaw(ctx, queryOperation, q, variables, name)
}

// MutateRaw executes a single GraphQL mutation request,
// with a mutation derived from m, populating the response into it.
// m should be a pointer to struct that corresponds to the GraphQL schema.
// return raw bytes message.
func (c *Client) MutateRaw(ctx context.Context, m interface{}, variables map[string]interface{}) (*json.RawMessage, error) {
	return c.doRaw(ctx, mutationOperation, m, variables, "")
}

// NamedMutateRaw executes a single GraphQL mutation request, with operation name
// return raw bytes message.
func (c *Client) NamedMutateRaw(ctx context.Context, name string, m interface{}, variables map[string]interface{}) (*json.RawMessage, error) {
	return c.doRaw(ctx, mutationOperation, m, variables, name)
}

// do executes a single GraphQL operation.
// return raw message and error
func (c *Client) doRaw(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}, name string) (*json.RawMessage, error) {
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

// do executes a single GraphQL operation and unmarshal json.
func (c *Client) do(ctx context.Context, op operationType, v interface{}, variables map[string]interface{}, name string) error {

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

		err := jsonutil.UnmarshalGraphQL(*out.Data, target, c.Strict)
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
