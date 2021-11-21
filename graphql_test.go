package graphql_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darrensapalo/go-graphql-client"
)

// Relies on json structural flags rather than graphql with support for json object
func TestClient_Query_NoGraphQLStructuralFlagsJSONObject(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(w,
			`{
  "data": {
    "node1": {
      "id": "MDEyOklzc3VlQ29tbWVudDE2OTQwNzk0Ng==",
			"someObject": {
				"test": "successful",
				"rly": "this is possible?",
				"even": {
					"nested": "objects?"
				}
			}
    },
    "node2": null
  }
}`)
	})
	client := graphql.NewClient("/graphql", &http.Client{Transport: localRoundTripper{handler: mux}})

	type (
		NodeA struct {
			ID         string      `json:"id"`
			SomeObject interface{} `json:"someObject"`
		}

		NodeB struct {
			ID string `json:"id"`
		}
	)

	var q struct {
		Node1 *NodeA `json:"node1,omitempty"`
		Node2 *NodeB `json:"node2"`
	}

	request := graphql.ManualRequest{
		Query:     "Does not matter",
		Variables: map[string]interface{}{},
		Result:    &q,
	}

	err := client.Query(context.Background(), request, nil)

	if err != nil {
		t.Fatalf("got error: %v, want: nil", err)
	}

	if q.Node1 == nil || q.Node1.ID != "MDEyOklzc3VlQ29tbWVudDE2OTQwNzk0Ng==" {
		t.Errorf("got wrong q.Node1: %+v", q.Node1)
	}

	if q.Node1.SomeObject == nil {
		t.Errorf("got nil q.Node1.SomeObject")
	}

	if x, ok := q.Node1.SomeObject.(map[string]interface{}); ok {
		fmt.Printf("OK %+v", x)
	}

	if q.Node2 != nil {
		t.Errorf("got non-nil q.Node2: %+v, want: nil", *q.Node2)
	}
}

func TestClient_Query_noDataWithErrorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(w, `{
      "errors": [
        {
          "message": "Field 'user' is missing required arguments: login",
          "locations": [
            {
              "line": 7,
              "column": 3
            }
          ]
        }
      ]
    }`)
	})
	client := graphql.NewClient("/graphql", &http.Client{Transport: localRoundTripper{handler: mux}})

	var q struct {
		User struct {
			Name graphql.String
		}
	}

	manualRequest := graphql.ManualRequest{
		Query:     "doesnt matter",
		Variables: make(map[string]interface{}),
		Result:    &q,
	}

	err := client.Query(context.Background(), manualRequest, nil)
	if err == nil {
		t.Fatal("got error: nil, want: non-nil")
	}
	if got, want := err.Error(), "Message: Field 'user' is missing required arguments: login, Locations: [{Line:7 Column:3}]"; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
	if q.User.Name != "" {
		t.Errorf("got non-empty q.User.Name: %v", q.User.Name)
	}
}

func TestClient_Query_errorStatusCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "important message", http.StatusInternalServerError)
	})
	client := graphql.NewClient("/graphql", &http.Client{Transport: localRoundTripper{handler: mux}})

	var q struct {
		User struct {
			Name graphql.String
		}
	}

	manualRequest := graphql.ManualRequest{
		Query:     "doesnt matter",
		Variables: make(map[string]interface{}),
		Result:    &q,
	}

	err := client.Query(context.Background(), manualRequest, nil)
	if err == nil {
		t.Fatal("got error: nil, want: non-nil")
	}
	if got, want := err.Error(), `non-200 OK status code: 500 Internal Server Error body: "important message\n"`; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
	if q.User.Name != "" {
		t.Errorf("got non-empty q.User.Name: %v", q.User.Name)
	}
}

// localRoundTripper is an http.RoundTripper that executes HTTP transactions
// by using handler directly, instead of going over an HTTP connection.
type localRoundTripper struct {
	handler http.Handler
}

func (l localRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	l.handler.ServeHTTP(w, req)
	return w.Result(), nil
}

func mustRead(r io.Reader) string {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func mustWrite(w io.Writer, s string) {
	_, err := io.WriteString(w, s)
	if err != nil {
		panic(err)
	}
}
