package iota

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

var (
	// Returned for 400 bad request HTTP responses.
	ErrHTTPBadRequest = errors.New("bad request")
	// Returned for 500 internal server error HTTP responses.
	ErrHTTPInternalServerError = errors.New("internal server error")
	// Returned for 404 not found error HTTP responses.
	ErrHTTPNotFound = errors.New("not found")
	// Returned for 401 unauthorized error HTTP responses.
	ErrHTTPUnauthorized = errors.New("unauthorized")
	// Returned for unknown error HTTP responses.
	ErrHTTPUnknownError = errors.New("unknown error")
	// Returned for 501 not implemented error HTTP responses.
	ErrHTTPNotImplemented = errors.New("operation not implemented/supported/available")

	httpCodeToErr = map[int]error{
		http.StatusBadRequest:          ErrHTTPBadRequest,
		http.StatusInternalServerError: ErrHTTPInternalServerError,
		http.StatusNotFound:            ErrHTTPNotFound,
		http.StatusUnauthorized:        ErrHTTPUnauthorized,
		http.StatusNotImplemented:      ErrHTTPNotImplemented,
	}
)

const (
	contentTypeJSON = "application/json"
)

// NewNodeAPI returns a new NodeAPI with the given BaseURL and HTTPClient.
func NewNodeAPI(baseURL string, httpClient ...http.Client) *NodeAPI {
	if len(httpClient) > 0 {
		return &NodeAPI{BaseURL: baseURL, HTTPClient: httpClient[0]}
	}
	return &NodeAPI{BaseURL: baseURL}
}

// NodeAPI is a client for node HTTP REST APIs.
type NodeAPI struct {
	// The HTTP client to use.
	HTTPClient http.Client
	// The base URL for all API calls.
	BaseURL string
}

// defines the error response schema for node API responses.
type httperrresponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// defines the ok response schema for node API responses.
type httpokresponse struct {
	Data interface{} `json:"data"`
}

func interpretBody(res *http.Response, decodeTo interface{}) error {
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("unable to read response body: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
		okRes := &httpokresponse{Data: decodeTo}
		return json.Unmarshal(resBody, okRes)
	}

	errRes := &httperrresponse{}
	if err := json.Unmarshal(resBody, errRes); err != nil {
		return fmt.Errorf("unable to read error from response body: %w", err)
	}

	err, ok := httpCodeToErr[res.StatusCode]
	if !ok {
		err = ErrHTTPUnknownError
	}

	return fmt.Errorf("%w: url %s, error message: %s", err, res.Request.URL.String(), errRes.Error.Message)
}

func (api *NodeAPI) do(method string, route string, reqObj interface{}, resObj interface{}) error {
	// marshal request object
	var data []byte
	if reqObj != nil {
		var err error
		data, err = json.Marshal(reqObj)
		if err != nil {
			return err
		}
	}

	// construct request
	req, err := http.NewRequest(method, fmt.Sprintf("%s%s", api.BaseURL, route), func() io.Reader {
		if data == nil {
			return nil
		}
		return bytes.NewReader(data)
	}())
	if err != nil {
		return err
	}

	if data != nil {
		req.Header.Set("Content-Type", contentTypeJSON)
	}

	// make the request
	res, err := api.HTTPClient.Do(req)
	if err != nil {
		return err
	}

	if resObj == nil {
		return nil
	}

	// write response into response object
	if err := interpretBody(res, resObj); err != nil {
		return err
	}
	return nil
}

// NodeInfoResponse defines the response of a node info HTTP API call.
type NodeInfoResponse struct {
	// The name of the node software.
	Name string `json:"name"`
	// The semver version of the node software.
	Version string `json:"version"`
	// Whether the node is healthy.
	IsHealthy bool `json:"isHealthy"`
	// The network in which the node operates in.
	OperatingNetwork string `json:"operatingNetwork"`
	// The amount of currently connected peers.
	Peers int `json:"peers"`
	// The used coordinator address.
	CoordinatorAddress string `json:"coordinatorAddress"`
	// Whether the node is synchronized.
	IsSynced bool `json:"isSynced"`
	// The latest known milestone hash.
	LatestMilestoneHash string `json:"latestMilestoneHash"`
	// The latest known milestone index.
	LatestMilestoneIndex uint64 `json:"latestMilestoneIndex"`
	// The current solid milestone's hash.
	LatestSolidMilestoneHash string `json:"latestSolidMilestoneHash"`
	// The current solid milestone's index.
	LatestSolidMilestoneIndex uint64 `json:"latestSolidMilestoneIndex"`
	// The milestone index at which the last pruning commenced.
	PruningIndex uint64 `json:"pruningIndex"`
	// The current time from the point of view of the node.
	Time uint64 `json:"time"`
	// The features this node exposes.
	Features []string `json:"features"`
}

// Info gets the info of the node.
func (api *NodeAPI) Info() (*NodeInfoResponse, error) {
	res := &NodeInfoResponse{}
	if err := api.do(http.MethodGet, "/info", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

// NodeTipsResponse defines the response of a node tips HTTP API call.
type NodeTipsResponse struct {
	// The hex encoded hash of the first tip message.
	Tip1 string `json:"tip1"`
	// The hex encoded hash of the second tip message.
	Tip2 string `json:"tip2"`
}

// Tips gets the two tips from the node.
func (api *NodeAPI) Tips() (*NodeTipsResponse, error) {
	res := &NodeTipsResponse{}
	if err := api.do(http.MethodGet, "/tips", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

// MessagesByHash gets messages by their hashes from the node.
func (api *NodeAPI) MessagesByHash(hashes MessageHashes) ([]*Message, error) {
	var query strings.Builder
	query.WriteString("/messages/by-hash?hashes=")
	query.WriteString(strings.Join(HashesToHex(hashes), ","))

	var res JSONNodeMessages
	if err := api.do(http.MethodGet, query.String(), nil, res); err != nil {
		return nil, err
	}

	return res.ToMessages()
}

// NodeObjectReferencedResponse defines the response for an object which is potentially
// referenced by a milestone node HTTP API call.
type NodeObjectReferencedResponse struct {
	// Tells whether the given object is referenced by a milestone.
	IsReferencedByMilestone bool `json:"isReferencedByMilestone"`
	// The index of the milestone which referenced the object.
	MilestoneIndex uint64 `json:"milestoneIndex"`
	// The timestamp of the milestone which referenced the object.
	MilestoneTimestamp uint64 `json:"milestoneTimestamp"`
}

// AreMessagesReferencedByMilestone tells whether the given messages are referenced by milestones.
// The response slice is ordered by the provided input hashes.
func (api *NodeAPI) AreMessagesReferencedByMilestone(hashes MessageHashes) ([]NodeObjectReferencedResponse, error) {
	var query strings.Builder
	query.WriteString("/messages/by-hash/is-referenced-by-milestone?hashes=")
	query.WriteString(strings.Join(HashesToHex(hashes), ","))

	var res []NodeObjectReferencedResponse
	if err := api.do(http.MethodGet, query.String(), nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

// AreTransactionsReferencedByMilestone tells whether the given transactions are referenced by milestones.
// The response slice is ordered by the provided input hashes.
func (api *NodeAPI) AreTransactionsReferencedByMilestone(hashes SignedTransactionPayloadHashes) ([]NodeObjectReferencedResponse, error) {
	var query strings.Builder
	query.WriteString("/transaction-messages/is-confirmed?hashes=")
	query.WriteString(strings.Join(HashesToHex(hashes), ","))

	var res []NodeObjectReferencedResponse
	if err := api.do(http.MethodGet, query.String(), nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

// NodeOutputResponse defines the construct of an output in a a node HTTP API call.
type NodeOutputResponse struct {
	// The address to which this output deposits to.
	Address string `json:"address"`
	// The amount of the deposit.
	Amount uint64 `json:"amount"`
	// Whether this output is spent.
	Spent bool `json:"spent"`
}

// OutputsByHash gets outputs by their ID from the node.
func (api *NodeAPI) OutputsByHash(utxosID UTXOInputIDs) ([]NodeOutputResponse, error) {
	var query strings.Builder
	query.WriteString("/outputs/by-hash?hashes=")
	query.WriteString(strings.Join(utxosID.ToHex(), ","))

	var res []NodeOutputResponse
	if err := api.do(http.MethodGet, query.String(), nil, res); err != nil {
		return nil, err
	}
	return res, nil
}