package testenv

import (
	"context"
	"errors"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type Kafka struct {
	Cleanup      func()
	CreateTopic  func(context.Context, string) error
	Username     string
	Password     string
	PlainBrokers []string
	AuthBrokers  []string
}

type kafkaConnConfig struct {
	plainBrokers []string
	authBrokers  []string
}

const (
	clusterID            = "TestEnvKafkaClusterID1"
	defaultUser          = "admin"
	defaultPassword      = "password"
	noAuthPort           = "9092/tcp"
	authPort             = "9093/tcp"
	starterScript        = "/usr/sbin/testcontainers_start.sh"
	starterScriptContent = `
export KAFKA_ADVERTISED_LISTENERS=%s,%s
# 1. Create the JAAS file dynamically
mkdir -p /tmp/kafka
cat <<EOF > /tmp/kafka/kafka_server_jaas.conf
KafkaServer {
    org.apache.kafka.common.security.scram.ScramLoginModule required
    username="%s"
    password="%s";
};
EOF

# 2. Create the Bootstrap Properties
cat <<EOF > /tmp/kafka/bootstrap.properties
early.start.listeners=CONTROLLER
process.roles=${KAFKA_PROCESS_ROLES}
node.id=${KAFKA_NODE_ID}
controller.quorum.voters=${KAFKA_CONTROLLER_QUORUM_VOTERS}
controller.listener.names=${KAFKA_CONTROLLER_LISTENER_NAMES}
listeners=${KAFKA_LISTENERS}
listener.security.protocol.map=${KAFKA_LISTENER_SECURITY_PROTOCOL_MAP}
inter.broker.listener.name=${KAFKA_INTER_BROKER_LISTENER_NAME}
sasl.enabled.mechanisms=${KAFKA_SASL_ENABLED_MECHANISMS}
sasl.mechanism.controller.protocol=${KAFKA_SASL_MECHANISM_CONTROLLER_PROTOCOL}
sasl.mechanism.inter.broker.protocol=${KAFKA_SASL_MECHANISM_INTER_BROKER_PROTOCOL}
EOF

# 3. Format if necessary
if [ ! -f /var/lib/kafka/data/meta.properties ]; then
    echo "Formatting storage...";
    /opt/kafka/bin/kafka-storage.sh format \
      -t %s \
      -c /tmp/kafka/bootstrap.properties \
      --add-scram "SCRAM-SHA-512=[name=%s,password=%s]";
fi

# 4. Start Kafka
exec /etc/kafka/docker/run
`
)

// NewKafka initializes or attaches to a reusable Kafka TestContainer.
func NewKafka(ctx context.Context, imageName string) (*Kafka, error) {
	// 1. Start Container
	container, err := testcontainers.GenericContainer(ctx, buildContainerRequest(imageName))
	if err != nil {
		return nil, fmt.Errorf("kafka container start failed: %w", err)
	}

	// 2. Extract Connection Info
	cfg, err := getKafkaConfig(ctx, container)
	if err != nil {
		return nil, err
	}

	// 3. Initialize Client
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.authBrokers...),
		kgo.SASL(scram.Auth{User: defaultUser, Pass: defaultPassword}.AsSha512Mechanism()),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client init failed: %w", err)
	}
	admin := kadm.NewClient(client)
	createTopic := func(testCtx context.Context, topic string) error {
		resp, err := admin.CreateTopics(testCtx, 1, 1, nil, topic)
		if err != nil {
			return fmt.Errorf("failed to create topic: %w", err)
		}

		if err := resp[topic].Err; err != nil && !errors.Is(err, kerr.TopicAlreadyExists) {
			return fmt.Errorf("topic error: %w", err)
		}
		return nil
	}

	return &Kafka{
		PlainBrokers: cfg.plainBrokers,
		AuthBrokers:  cfg.authBrokers,
		Username:     defaultUser,
		Password:     defaultPassword,
		CreateTopic:  createTopic,
		Cleanup:      func() {}, // Managed by Ryuk
	}, nil
}

func buildContainerRequest(imageName string) testcontainers.GenericContainerRequest {
	reuseName := SanitizeContainerName("kafka", imageName)

	return testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        imageName,
			Env:          getKafkaEnv(),
			ExposedPorts: []string{noAuthPort, authPort, "9094/tcp"},
			Entrypoint:   []string{"/bin/sh"},
			Cmd:          []string{"-c", "while [ ! -f " + starterScript + " ]; do sleep 0.1; done; bash " + starterScript},
			LifecycleHooks: []testcontainers.ContainerLifecycleHooks{
				{
					PostStarts: []testcontainers.ContainerHook{
						injectStarterScript,
					},
				},
			},
			Name: reuseName,
		},
		Started: true,
		Reuse:   true,
	}
}

func getKafkaEnv() map[string]string {
	return map[string]string{
		"CLUSTER_ID":                                     clusterID,
		"KAFKA_NODE_ID":                                  "1",
		"KAFKA_PROCESS_ROLES":                            "broker,controller",
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "CONTROLLER:SASL_PLAINTEXT,NO_AUTH:PLAINTEXT,AUTH:SASL_PLAINTEXT",
		"KAFKA_LISTENERS":                                "NO_AUTH://:9092,AUTH://:9093,CONTROLLER://:9094",
		"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
		"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@127.0.0.1:9094",
		"KAFKA_INTER_BROKER_LISTENER_NAME":               "AUTH",
		"KAFKA_SASL_ENABLED_MECHANISMS":                  "SCRAM-SHA-512",
		"KAFKA_SASL_MECHANISM_CONTROLLER_PROTOCOL":       "SCRAM-SHA-512",
		"KAFKA_SASL_MECHANISM_INTER_BROKER_PROTOCOL":     "SCRAM-SHA-512",
		"KAFKA_OPTS":                                     "-Djava.security.auth.login.config=/tmp/kafka/kafka_server_jaas.conf",
		"KAFKA_AUTHORIZER_CLASS_NAME":                    "org.apache.kafka.metadata.authorizer.StandardAuthorizer",
		"KAFKA_SUPER_USERS":                              fmt.Sprintf("User:%s", defaultUser),
		"KAFKA_ALLOW_EVERYONE_IF_NO_ACL_FOUND":           "true",
		"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
		"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
		"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
		"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
	}
}

func injectStarterScript(ctx context.Context, c testcontainers.Container) error {
	plainEndpoint, err := c.PortEndpoint(ctx, noAuthPort, "NO_AUTH")
	if err != nil {
		return fmt.Errorf("endpoint %s: %w", noAuthPort, err)
	}
	authEndpoint, err := c.PortEndpoint(ctx, authPort, "AUTH")
	if err != nil {
		return fmt.Errorf("endpoint %s: %w", authPort, err)
	}

	finalScript := fmt.Sprintf(
		starterScriptContent,
		plainEndpoint,
		authEndpoint,
		defaultUser,
		defaultPassword,
		clusterID,
		defaultUser,
		defaultPassword,
	)

	err = c.CopyToContainer(ctx, []byte(finalScript), starterScript, 0o755)
	if err != nil {
		return fmt.Errorf("failed to copy starter script to kafka container: %w", err)
	}

	return wait.
		ForLog(".*Kafka Server started.*").
		AsRegexp().
		WaitUntilReady(ctx, c)
}

func getKafkaConfig(ctx context.Context, c testcontainers.Container) (*kafkaConnConfig, error) {
	host, err := c.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host: %w", err)
	}
	dockerNoAuthPort, err := c.MappedPort(ctx, noAuthPort)
	if err != nil {
		return nil, fmt.Errorf("failed to map port %v: %w", noAuthPort, err)
	}
	dockerAuthPort, err := c.MappedPort(ctx, authPort)
	if err != nil {
		return nil, fmt.Errorf("failed to map port %v: %w", authPort, err)
	}

	return &kafkaConnConfig{
		plainBrokers: []string{fmt.Sprintf("%s:%s", host, dockerNoAuthPort.Port())},
		authBrokers:  []string{fmt.Sprintf("%s:%s", host, dockerAuthPort.Port())},
	}, nil
}
