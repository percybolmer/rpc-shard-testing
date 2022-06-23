package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"percybolmer/rpc-shard-testing/rpctester/contracts/devtoken"
	"percybolmer/rpc-shard-testing/rpctester/crypto"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)

const (
	METHOD_V1_getBalanceByBlockNumber = "hmy_getBalanceByBlockNumber"
	METHOD_V2_getBalanceByBlockNumber = "hmyv2_getBalanceByBlockNumber"
	METHOD_V1_getTransactionCount     = "hmy_getTransactionCount"
	METHOD_V2_getTransactionCount     = "hmyv2_getTransactionCount"
	METHOD_V1_getBalance              = "hmy_getBalance"
	METHOD_V2_getBalance              = "hmyv2_getBalance"
	METHOD_address                    = "address"
	/**
	Filter related methods

	*/
	METHOD_filter_getFilterLogs               = "hmy_getFilterLogs"
	METHOD_filter_newFilter                   = "hmy_newFilter"
	METHOD_filter_newPendingtransactionFilter = "hmy_newPendingTransactionFilter"
	METHOD_filter_newBlockFilter              = "hmy_newBlockFilter"
	METHOD_filter_getFilterChanges            = "hmy_getFilterChanges"
	METHOD_filter_getLogs                     = "hmy_getLogs"
	/*
		transactions related methods,
		Get help with the staking transactions
		Since they all fail,
	*/
	METHOD_transaction_V1_getStakingTransactionByBlockHashAndIndex   = "hmy_getStakingTransactionByBlockHashAndIndex"
	METHOD_transaction_V2_getStakingTransactionByBlockHashAndIndex   = "hmyv2_getStakingTransactionByBlockHashAndIndex"
	METHOD_transaction_V1_getStakingTransactionByBlockNumberAndIndex = "hmy_getStakingTransactionByBlockNumberAndIndex"
	METHOD_transaction_V2_getStakingTransactionByBlockNumberAndIndex = "hmyv2_getStakingTransactionByBlockNumberAndIndex"
	METHOD_transaction_V1_getStakingTransactionByHash                = "hmy_getStakingTransactionByHash"
	METHOD_transaction_V2_getStakingTransactionByHash                = "hmyv2_getStakingTransactionByHash"
	METHOD_transaction_V1_getCurrentTransactionErrorSink             = "hmy_getCurrentTransactionErrorSink"
	METHOD_transaction_V2_getCurrentTransactionErrorSink             = "hmyv2_getCurrentTransactionErrorSink"
	//
	METHOD_transaction_V1_getPendingCrossLinks = "hmy_getPendingCrossLinks"
	METHOD_transaction_V2_getPendingCrossLinks = "hmyv2_getPendingCrossLinks"
	METHOD_transaction_V1_getPendingCXReceipts = "hmy_getPendingCXReceipts"
	METHOD_transaction_V2_getPendingCXReceipts = "hmyv2_getPendingCXReceipts"
	METHOD_transaction_V1_getCXReceiptByHash   = "hmy_getCXReceiptByHash"
	METHOD_transaction_V2_getCXReceiptByHash   = "hmyv2_getCXReceiptByHash"
	METHOD_transaction_V1_pendingTransactions  = "hmy_pendingTransactions"
	METHOD_transaction_V2_pendingTransactions  = "hmyv2_pendingTransactions"
	// TODO How do I format the RawStaking Transaction?
	METHOD_transaction_sendRawStakingTransaction = "hmy_sendRawStakingTransaction"
	METHOD_transaction_sendRawTransaction        = "hmy_sendRawTransaction"
	// Different endpoints?
	METHOD_transaction_V1_getTransactionHistory = "hmy_getTransactionsHistory"
	METHOD_transaction_V2_getTransactionHistory = "hmyv2_getTransactionsHistory"
	METHOD_transaction_V1_getTransactionReceipt = "hmy_getTransactionReceipt"
	METHOD_transaction_V2_getTransactionReceipt = "hmyv2_getTransactionReceipt"
)

var (
	httpClient    *http.Client
	testMetrics   []TestMetric
	ethClient     *ethclient.Client
	auth          *bind.TransactOpts
	deployedToken *devtoken.Devtoken

	address                     string
	url                         string
	smartContractDeploymentHash string
	smartContractAddress        common.Address
)

func init() {
	// Dont like global http Client, but in this case it might make somewhat sense since we only want to test
	httpClient = &http.Client{Timeout: time.Duration(5) * time.Second}
	testMetrics = []TestMetric{}

	// Load .dotenv file
	environment := os.Getenv("RPCTESTER_ENVIRONMENT")

	if environment == "production" {
		if err := godotenv.Load(".env-prod"); err != nil {
			log.Fatal("Error loading .env-prod file")
		}
	} else {
		if err := godotenv.Load(".env"); err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	address = os.Getenv("ADDRESS")
	url = os.Getenv("NET_URL")
	smartContractAddr := os.Getenv("SMART_CONTRACT_ADDRESS")
	smartContractDeploymentHash = os.Getenv("SMART_CONTRACT_DEPLOY_HASH")
	// Create eth client
	ethClient, auth = crypto.NewClient()
	// Load the Smart Contract
	smartContractAddress = common.HexToAddress(smartContractAddr)
	instance, err := devtoken.NewDevtoken(smartContractAddress, ethClient)
	if err != nil {
		log.Fatal(err)
	}
	deployedToken = instance

}

// TypeResults is the results of all the tests
type TestResults struct {
	AddressUsed string       `json:"addressUsed"`
	Network     string       `json:"network"`
	Metrics     []TestMetric `json:"metrics"`
}

type TestMetric struct {
	Method   string        `json:"method"`
	Test     string        `json:"test"`
	Pass     bool          `json:"pass"`
	Duration string        `json:"duration"`
	Error    string        `json:"error,omitempty"`
	Params   []interface{} `json:"params,omitempty"`
}

// BaseRequest is the base structure of requests
type BaseRequest struct {
	ID      string `json:"id"`
	JsonRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	// Params holds the arguments
	Params []interface{} `json:"params"`
}

// BaseResponse is the base RPC response, but added extra metrics
type BaseResponse struct {
	ID      string          `json:"id"`
	JsonRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	// Error is only present when errors occurs
	Error *RPCError `json:"error,omitempty"`
	// Custom data fields not part of rpc response
	Method string `json:"method"`
	// Not part of the default message
	Duration string `json:"duration"`
}

type AddressResponse struct {
	ID           string        `json:"id"`
	Balance      big.Int       `json:"balance"`
	Transactions []Transaction `json:"txs" `
	// Custom data fields not part of rpc response
	Method string `json:"method"`
	// Not part of the default message
	Duration string `json:"duration"`
}

type RPCError struct {
	Code    int64
	Message string
}

func GenerateReport() {
	results := TestResults{
		AddressUsed: address,
		Network:     url,
		Metrics:     testMetrics,
	}
	// After all tests, Generate report
	data, err := json.Marshal(results)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("results.json", data, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
}

// Call will trigger a request with a payload to the RPC method given and marshal response into interface
func Call(payload []byte, method string) (*BaseResponse, error) {
	// Store request time in Response

	start := time.Now()
	resp, err := httpClient.Post(fmt.Sprintf("%s", url), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	duration := time.Since(start)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	// Read data
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var br BaseResponse
	err = json.Unmarshal(body, &br)
	if err != nil {
		return nil, err
	}
	// Add Extra metrics
	br.Duration = duration.String()
	br.Method = method

	return &br, nil
}

// Addres fetches address, this does not work as the other rpc calls
func Address(id string, offset, page int, tx_view string) (*AddressResponse, error) {
	client := &http.Client{}

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/address", url), nil)
	req.Header.Add("Content-Type", "application/json")

	q := req.URL.Query()
	q.Add("id", id)
	q.Add("offset", strconv.Itoa(offset))
	q.Add("page", strconv.Itoa(page))
	q.Add("tx_view", tx_view)
	req.URL.RawQuery = q.Encode()
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	duration := time.Since(start)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	// Read data
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response AddressResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	response.Method = METHOD_address
	response.Duration = duration.String()

	return &response, nil
}
