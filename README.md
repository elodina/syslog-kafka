[![Build Status](https://travis-ci.org/elodina/syslog-kafka.svg?branch=master)](https://travis-ci.org/elodina/syslog-kafka)

Syslog Producer for Apache Kafka 
==================================

This tool setups a Syslog server listening for syslog messages via TCP, parses them and produces into a Kafka topic.

**Usage**:

`go run syslog.go --broker.list localhost:9092 --topic syslog`

**Configuration parameters**:

`--broker.list` - Kafka broker list host:port to discover cluster. *This parameter is required*.    
`--topic` - destination topic for all incoming syslog messages. *This parameter is required*.    
`--tcp.port` - TCP port to listen for incoming syslog messages. *Defaults to 5140*.    
`--tcp.host` - TCP host to listen for incoming syslog messages. *Defaults to 0.0.0.0*.    
`--udp.port` - UDP port to listen for incoming syslog messages. *Defaults to 5141*.     
`--udp.host` - UDP host to listen for incoming syslog messages. *Defaults to 0.0.0.0*.     
`--num.producers` - number of producer instances. This can be used to increase throughput. *Defaults to 1*.    
`--required.acks` - required acks for producer. `0` - no server response. `1` - the server will wait the data is written to the local log. `-1` - the server will block until the message is committed by all in sync replicas. More on this [here](https://cwiki.apache.org/confluence/display/KAFKA/A+Guide+To+The+Kafka+Protocol#AGuideToTheKafkaProtocol-ProduceRequest). *Defaults to 1*.    
`--acks.timeout` - provides a maximum time in milliseconds the server can await the receipt of the number of acknowledgements in `--required.acks`. *Defaults to 1000*.     
`--avro` - Flag to send all messages as Avro LogLine. *Defaults to false*.    
`--schema.registry.url` - Avro Schema Registry url. Required for with --avro flag.    
`--proto` - Flag to send all messages as Protocol Buffers. *Defaults to false*.    
`--tag` - Arbitrary key=value tag that will be appended to message. Multiple of these flags allowed.    
`--log.type.id` - Log type ID to set to Avro or Protobuf messages.    

Running with Docker
==================

We provided a Docker image for quick deployment without cloning the repo, building the binary etc. Running it is as easy as following:

`docker run --net=host elodina/syslog-kafka --topic syslog --broker.list host:port`

You may pass all configurations described above as arguments to the container.

Try it out using Vagrant
=======================

1. `cd $GOPATH/src/github.com/elodina/syslog-kafka`
2. `vagrant up`

After this is done you will have a VM available at `192.168.66.66` with Zookeeper on port 2181, Kafka on port 9092, and Syslog Server listening on TCP port 5140 and UDP port 5141. To verify this is working you may do the following:

1. Start the console consumer: `$KAFKA_HOME/bin/kafka-console-consumer.sh --zookeeper 192.168.66.66:2181 --topic syslog --from-beginning`
2. Send some syslog messages via console: `echo "<34>1 2003-10-11T22:14:15.003Z localhost.elodina.net su - ID23 - a simple message" > /dev/tcp/192.168.66.66/5140` to send via TCP or `echo "<34>1 2003-10-11T22:14:15.003Z localhost.elodina.net su - ID23 - a simple message" > /dev/udp/192.168.66.66/5141` to send via UDP.

**Customizing server**:

You may want to produce to different topic or listen to other ports than 5140 and 5141 etc. All settings are kept in `vagrant/syslog.sh` file and can be modified before `vagrant up` to customize configurations.
