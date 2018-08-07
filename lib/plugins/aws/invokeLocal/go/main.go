package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	lc "github.com/aws/aws-lambda-go/lambdacontext"
	gli "github.com/djhworld/go-lambda-invoke/golambdainvoke"
)

type lambdaInput struct {
	Event   interface{}       `json:"event"`
	Context *lc.ClientContext `json:"context"`
}

var portNum = 8001
var port = strconv.Itoa(portNum)

// Take the serialized JSON from
func splitSlsInput(input lambdaInput) ([]byte, []byte, error) {
	payloadEncoded, err := json.Marshal(input.Event)
	if err != nil {
		return nil, nil, err
	}
	clientContextEncoded, err := json.Marshal(input.Context)
	if err != nil {
		return nil, nil, err
	}

	return payloadEncoded, clientContextEncoded, nil
}

// Poll every second for 120s for net/rpc in lambda.Start to setup
func waitUntilPortOpen() error {
	start := time.Now()
	hostPort := net.JoinHostPort("", port)
	for {
		conn, _ := net.DialTimeout("tcp", hostPort, 1*time.Second)
		if conn != nil {
			conn.Close()
			break
		}
		if time.Since(start) > 120*time.Second {
			return errors.New("timeout waiting for lambda RPC handler TCP server listen")
		}
	}
	return nil
}

// Execute the binary from provided path
func execLambdaBin(binaryPath string) (*exec.Cmd, error) {
	binary, err := exec.LookPath(binaryPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(binary)
	env := os.Environ()
	env = append(env, fmt.Sprintf("_LAMBDA_SERVER_PORT=%d", portNum))
	cmd.Env = env

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

// Ensure the port is open
func validateRPCPort() error {
	ln, err := net.Listen("tcp", ":"+port)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't listen on port %d: %s", portNum, err)
		os.Exit(10)
	}

	return ln.Close()
}

func main() {
	err := validateRPCPort()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(11)
	}

	binaryPath := os.Args[1]
	cmd, err := execLambdaBin(binaryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(20)
		return
	}
	defer cmd.Process.Kill()

	err = waitUntilPortOpen()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(21)
		return
	}

	in := os.Stdin
	scanner := bufio.NewScanner(in)
	var input lambdaInput
	for scanner.Scan() {
		// serverless pipes serialized json input to stdin
		rawInput := scanner.Bytes()
		fmt.Printf("Input: %s", rawInput)
		err := json.Unmarshal(rawInput, &input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		res, err := gli.RunWithClientContext(portNum, input.Event, input.Context)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		fmt.Printf("Received Response: %s", res)
	}
}
