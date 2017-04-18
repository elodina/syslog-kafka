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

package syslog

import (
	"bufio"
	"github.com/elodina/siesta"
	"github.com/elodina/siesta-producer"
	"github.com/golang/glog"
	"net"
	"os"
	"strings"
	"time"
)

type SyslogMessage struct {
	Message   string
	Hostname  string
	Timestamp int64
}

// SyslogProducerConfig defines configuration options for SyslogProducer
type SyslogProducerConfig struct {
	// Syslog producer config.
	ProducerConfig *producer.ProducerConfig

	// Number of producer instances.
	NumProducers int

	Topic string

	// Receive messages from this TCP address and post them to topic.
	TCPAddr string

	// Receive messages from this UDP address and post them to topic.
	UDPAddr string

	// Kafka Broker List host:port,host:port
	BrokerList string

	// Hostname the message came from
	Hostname string

	Tags map[string]string

	// Transformer func(message syslogparser.LogParts, topic string) *sarama.ProducerMessage
	Transformer func(message *SyslogMessage, topic string) *producer.ProducerRecord

	ValueSerializer producer.Serializer
}

// Creates an empty SyslogProducerConfig.
func NewSyslogProducerConfig() *SyslogProducerConfig {
	hostname, err := os.Hostname()
	if err != nil {
		glog.Warningf("Failed to get hostname, using blank source value: %s", err)
	}
	return &SyslogProducerConfig{
		Hostname:        hostname,
		Transformer:     simpleTransformFunc,
		ValueSerializer: producer.StringSerializer,
	}
}

type SyslogProducer struct {
	config        *SyslogProducerConfig
	incoming      chan *SyslogMessage
	closeChannels []chan bool

	producers []*producer.KafkaProducer
}

func NewSyslogProducer(config *SyslogProducerConfig) *SyslogProducer {
	return &SyslogProducer{
		config:   config,
		incoming: make(chan *SyslogMessage),
	}
}

func (this *SyslogProducer) String() string {
	return "syslog-producer"
}

func (this *SyslogProducer) Start() {
	glog.V(2).Info("Starting syslog producer")
	this.startTCPServer()
	this.startUDPServer()
	this.startProducers()
}

func (this *SyslogProducer) Stop() {
	glog.V(2).Info("Stopping syslog producer")

	for _, closeChannel := range this.closeChannels {
		closeChannel <- true
	}
	close(this.incoming)

	for _, producer := range this.producers {
		producer.Close()
	}
}

func (this *SyslogProducer) startTCPServer() {
	glog.V(2).Info("Starting TCP server")
	tcpAddr, err := net.ResolveTCPAddr("tcp", this.config.TCPAddr)
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		panic(err)
	}
	closeChannel := make(chan bool, 1)
	this.closeChannels = append(this.closeChannels, closeChannel)

	go func() {
		for {
			select {
			case <-closeChannel:
				return
			default:
			}
			connection, err := listener.Accept()
			if err != nil {
				return
			}

			this.scan(connection)
		}
	}()
	glog.Infof("Listening for messages at TCP %s", this.config.TCPAddr)
}

func (this *SyslogProducer) startUDPServer() {
	glog.V(2).Info("Starting UDP server")
	udpAddr, err := net.ResolveUDPAddr("udp", this.config.UDPAddr)
	if err != nil {
		panic(err)
	}

	connection, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(err)
	}
	closeChannel := make(chan bool, 1)
	this.closeChannels = append(this.closeChannels, closeChannel)

	go func() {
		for {
			select {
			case <-closeChannel:
				return
			default:
			}

			this.scan(connection)
		}
	}()
	glog.Infof("Listening for messages at UDP %s", this.config.UDPAddr)
}

func (this *SyslogProducer) scan(connection net.Conn) {
	scanner := bufio.NewScanner(connection)
	for scanner.Scan() {
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		this.incoming <- &SyslogMessage{scanner.Text(), this.config.Hostname, timestamp}
	}
}

func (this *SyslogProducer) startProducers() {
	brokerList := strings.Split(this.config.BrokerList, ",")

	connectorConfig := siesta.NewConnectorConfig()
	connectorConfig.BrokerList = brokerList
	connector, err := siesta.NewDefaultConnector(connectorConfig)
	if err != nil {
		panic(err)
	}

	for i := 0; i < this.config.NumProducers; i++ {
		glog.V(2).Infof("Starting new producer with config: %#v", this.config.ProducerConfig)
		producer := producer.NewKafkaProducer(this.config.ProducerConfig, producer.ByteSerializer, this.config.ValueSerializer, connector)
		this.producers = append(this.producers, producer)
		go this.produceRoutine(producer)
	}
}

func (this *SyslogProducer) produceRoutine(producer *producer.KafkaProducer) {
	for msg := range this.incoming {
		producer.Send(this.config.Transformer(msg, this.config.Topic))
	}
}

func simpleTransformFunc(msg *SyslogMessage, topic string) *producer.ProducerRecord {
	return &producer.ProducerRecord{Topic: topic, Value: msg.Message}
}
