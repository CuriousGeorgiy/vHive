// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	. "github.com/ease-lab/vhive/examples/endpoint"
	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

const TimeseriesDBAddr = "10.96.0.84:90"

var (
	completed   int64
	latSlice    LatencySlice
	portFlag    *int
	withTracing *bool
)

func main() {
	endpointsFile := flag.String("endpointsFile", "endpoints.json", "File with endpoints' metadata")
	rps := flag.Int("rps", 1, "Target requests per second")
	runDuration := flag.Int("time", 5, "Run each benchmark for X seconds")
	latencyOutputFile := flag.String("latf", "lat.csv", "CSV file for the latency measurements in microseconds")
	portFlag = flag.Int("port", 80, "The port that functions listen to")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
	zipkin := flag.String("zipkin", "http://localhost:9411/api/v2/spans", "zipkin url")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)
	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	log.Info("Reading the endpoints from the file: ", *endpointsFile)

	endpoints, err := readEndpoints(*endpointsFile)
	if err != nil {
		log.Fatal("Failed to read the Hostname files: ", err)
	}

	if *withTracing {
		shutdown, err := tracing.InitBasicTracer(*zipkin, "invoker")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}

	realRPS := runBenchmark(endpoints, *runDuration, *rps)

	writeLatencies(realRPS, *latencyOutputFile)
}

func readEndpoints(path string) (endpoints []Endpoint, _ error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &endpoints); err != nil {
		return nil, err
	}
	return
}

func runBenchmark(endpoints []Endpoint, runDuration, targetRPS int) (realRPS float64) {
	for i, endpoint := range endpoints {
		if endpoint.Eventing {
			Start(TimeseriesDBAddr, endpoints[0].Matchers)
		}

		issued := 0
		start := time.Now()
		timeout := time.After(time.Duration(runDuration) * time.Second)
		tick := time.Tick(time.Duration(1000/targetRPS) * time.Millisecond)

		burst:
		for {
			if endpoint.Eventing {
				go invokeEventingFunction(endpoint)
			} else {
				go invokeServingFunction(endpoint.Hostname)
			}
			issued++

			select {
			case <-timeout:
				duration := time.Since(start).Seconds()
				realRPS = float64(completed) / duration
				log.Infof("Endpoint #%d", i)
				log.Infof("  Issued / completed requests: %d, %d", issued, completed)
				log.Infof("  Real / target RPS: %.2f / %v", realRPS, targetRPS)
				break burst
			case <-tick:
				continue
			}
		}

		if endpoint.Eventing {
			addDurations(End())
		}
	}

	log.Println("Benchmark finished!")
	return
}

func SayHello(address string) {
	var dialOption grpc.DialOption
	if *withTracing {
		dialOption = grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())
	} else {
		dialOption = grpc.WithBlock()
	}
	conn, err := grpc.Dial(address, grpc.WithInsecure(), dialOption)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = c.SayHello(ctx, &pb.HelloRequest{Name: "faas"})
	if err != nil {
		log.Warnf("Failed to invoke %v, err=%v", address, err)
	}
}

func invokeEventingFunction(endpoint Endpoint) {
	address := fmt.Sprintf("%s:%d", endpoint.Hostname, *portFlag)
	log.Debug("Invoking by the address: %v", address)

	SayHello(address)

	atomic.AddInt64(&completed, 1)

	return
}

func invokeServingFunction(hostname string) {
	defer getDuration(startMeasurement(hostname)) // measure entire invocation time

	address := fmt.Sprintf("%s:%d", hostname, *portFlag)
	log.Debug("Invoking by the address: %v", address)

	SayHello(address)

	atomic.AddInt64(&completed, 1)

	return
}

// LatencySlice is a thread-safe slice to hold a slice of latency measurements.
type LatencySlice struct {
	sync.Mutex
	slice []int64
}

func startMeasurement(msg string) (string, time.Time) {
	return msg, time.Now()
}

func getDuration(msg string, start time.Time) {
	latency := time.Since(start)
	log.Debugf("Invoked %v in %v usec\n", msg, latency)
	addDurations([]time.Duration{latency})
}

func addDurations(ds []time.Duration) {
	latSlice.Lock()
	for _, d := range ds {
		latSlice.slice = append(latSlice.slice, d.Microseconds())
	}
	latSlice.Unlock()
}

func writeLatencies(rps float64, latencyOutputFile string) {
	latSlice.Lock()
	defer latSlice.Unlock()

	fileName := fmt.Sprintf("rps%.2f_%s", rps, latencyOutputFile)
	log.Info("The measured latencies are saved in ", fileName)

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		log.Fatal("Failed creating file: ", err)
	}

	datawriter := bufio.NewWriter(file)

	for _, lat := range latSlice.slice {
		_, err := datawriter.WriteString(strconv.FormatInt(lat, 10) + "\n")
		if err != nil {
			log.Fatal("Failed to write the URLs to a file ", err)
		}
	}

	datawriter.Flush()
	file.Close()
	return
}
