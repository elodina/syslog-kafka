/* Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License. */

package main

import (
	"errors"
	"flag"
	"fmt"
	kafkaavro "github.com/elodina/go-kafka-avro"
	producer "github.com/elodina/siesta-producer"
	"github.com/elodina/syslog-kafka/avro"
	sp "github.com/elodina/syslog-kafka/proto"
	"github.com/elodina/syslog-kafka/syslog"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"math"
	"os"
	"os/signal"
	"strings"
	"time"
)

type tags map[string]string

func (i tags) String() string {
	tagsSlice := make([]string, len(i))
	idx := 0
	for k, v := range i {
		tagsSlice[idx] = fmt.Sprintf("%s=%s", k, v)
		idx++
	}

	return strings.Join(tagsSlice, ",")
}

func (i tags) Set(value string) error {
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return errors.New("Tags are expected in k=v format.")
	}

	i[kv[0]] = kv[1]
	return nil
}

var numProducers int
var topic string
var tcpPort int
var tcpHost string
var udpPort int
var udpHost string
var brokerList string
var requiredAcks int
var acksTimeout int
var schemaRegistryUrl string
var sendAvro bool
var sendProtobuf bool

//additional params
var tag tags = make(tags)
var logtypeid int64

func init() {
	flag.IntVar(&numProducers, "num.producers", 1, "Number of producers.")
	flag.StringVar(&topic, "topic", "", "Topic to produce messages into.")
	flag.IntVar(&tcpPort, "tcp.port", 5140, "TCP port to listen for incoming messages.")
	flag.StringVar(&tcpHost, "tcp.host", "0.0.0.0", "TCP host to listen for incoming messages.")
	flag.IntVar(&udpPort, "udp.port", 5141, "UDP port to listen for incoming messages.")
	flag.StringVar(&udpHost, "udp.host", "0.0.0.0", "UDP host to listen for incoming messages.")
	flag.StringVar(&brokerList, "broker.list", "", "Broker List to produce messages too.")
	flag.IntVar(&requiredAcks, "required.acks", 1, "Required acks for producer. 0 - no server response. 1 - the server will wait the data is written to the local log. -1 - the server will block until the message is committed by all in sync replicas.")
	flag.IntVar(&acksTimeout, "acks.timeout", 1000, "This provides a maximum time in milliseconds the server can await the receipt of the number of acknowledgements in RequiredAcks.")
	flag.StringVar(&schemaRegistryUrl, "schema.registry.url", "", "Avro Schema Registry url. Required for with --avro flag.")
	flag.BoolVar(&sendAvro, "avro", false, "Flag to send all messages as Avro LogLine.")
	flag.BoolVar(&sendProtobuf, "proto", false, "Flag to send all messages as Protocol Buffers.")
	flag.Var(tag, "tag", "Arbitrary key=value tag that will be appended to message. Multiple of these flags allowed.")
	flag.Int64Var(&logtypeid, "log.type.id", math.MinInt64, "Log type ID to set to Avro or Protobuf messages.")
}

func validate() *syslog.SyslogProducerConfig {
	if brokerList == "" {
		fmt.Println("broker.list is required.")
		os.Exit(1)
	}

	if topic == "" {
		fmt.Println("Topic is required.")
		os.Exit(1)
	}

	if sendAvro && schemaRegistryUrl == "" {
		fmt.Println("Schema Registry URL is required for --avro flag")
		os.Exit(1)
	}

	config := syslog.NewSyslogProducerConfig()
	config.ProducerConfig = producer.NewProducerConfig()
	config.ProducerConfig.RequiredAcks = requiredAcks
	config.ProducerConfig.AckTimeoutMs = int32(acksTimeout)
	config.BrokerList = brokerList
	config.NumProducers = numProducers
	config.Topic = topic
	config.TCPAddr = fmt.Sprintf("%s:%d", tcpHost, tcpPort)
	config.UDPAddr = fmt.Sprintf("%s:%d", udpHost, udpPort)

	if sendAvro {
		serializer := kafkaavro.NewKafkaAvroEncoder(schemaRegistryUrl)
		config.ValueSerializer = serializer.Encode
		config.Transformer = avroTransformer
	}

	if sendProtobuf {
		config.ValueSerializer = producer.ByteSerializer
		config.Transformer = protobufTransformer
	}

	return config
}

func main() {
	flag.Parse()
	config := validate()

	producer := syslog.NewSyslogProducer(config)
	go producer.Start()

	ctrlc := make(chan os.Signal, 1)
	signal.Notify(ctrlc, os.Interrupt)
	<-ctrlc
	producer.Stop()
}

func avroTransformer(msg *syslog.SyslogMessage, topic string) *producer.ProducerRecord {
	logLine := avro.NewLogLine()
	logLine.Line = msg.Message
	logLine.Source = msg.Hostname
	logLine.Tag = tag
	if logtypeid != math.MinInt64 {
		logLine.Logtypeid = logtypeid
	}
	logLine.Timings = make([]*avro.Timing, 0)
	logLine.Timings = append(logLine.Timings, &avro.Timing{
		EventName: "received",
		Value:     msg.Timestamp,
	})

	return &producer.ProducerRecord{Topic: topic, Value: logLine}
}

func protobufTransformer(msg *syslog.SyslogMessage, topic string) *producer.ProducerRecord {
	line := &sp.LogLine{}

	line.Line = proto.String(msg.Message)
	line.Source = proto.String(msg.Hostname)
	for k, v := range tag {
		line.Tag = append(line.Tag, &sp.LogLine_Tag{Key: proto.String(k), Value: proto.String(v)})
	}
	if logtypeid != math.MinInt64 {
		line.Logtypeid = proto.Int64(logtypeid)
	}
	line.Timings = append(line.Timings, msg.Timestamp, time.Now().UnixNano()/int64(time.Millisecond))

	protobuf, err := proto.Marshal(line)
	if err != nil {
		glog.Errorf("Failed to marshal %s as Protocol Buffer", msg)
	}

	return &producer.ProducerRecord{Topic: topic, Value: protobuf}
}
