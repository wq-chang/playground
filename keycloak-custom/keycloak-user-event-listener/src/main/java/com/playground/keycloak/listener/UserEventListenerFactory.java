package com.playground.keycloak.listener;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.playground.keycloak.publisher.EventPublisher;
import com.playground.keycloak.publisher.KafkaEventPublisherImpl;
import com.playground.keycloak.service.UserEventServiceImpl;
import com.playground.keycloak.util.EventLogger;
import java.util.Properties;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.common.serialization.ByteArraySerializer;
import org.apache.kafka.common.serialization.StringSerializer;
import org.jboss.logging.Logger;
import org.keycloak.Config;
import org.keycloak.events.EventListenerProvider;
import org.keycloak.events.EventListenerProviderFactory;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;
import org.keycloak.utils.StringUtil;

public class UserEventListenerFactory implements EventListenerProviderFactory {

  private static final Logger logger = Logger.getLogger(UserEventListenerFactory.class);
  private static final String PROVIDER_ID = "user-event-listener";
  private EventPublisher publisher;
  private EventLogger eventLogger;

  @Override
  public void init(Config.Scope config) {
    logger.info("Initializing UserEventListener");

    publisher = buildPublisher(config);
    eventLogger = new EventLogger();
  }

  @Override
  public EventListenerProvider create(KeycloakSession session) {
    logger.debug("Creating UserEventListener instance");
    UserEventServiceImpl service = new UserEventServiceImpl(eventLogger, publisher);

    return new UserEventListener(service);
  }

  @Override
  public void postInit(KeycloakSessionFactory factory) {
    logger.info("Post-initializing UserEventListener");
  }

  @Override
  public void close() {
    logger.info("Closing UserEventListenerFactory");
    if (publisher != null) {
      try {
        publisher.close();
      } catch (Exception e) {
        logger.error("Failed to close publisher", e);
      }
    }
  }

  @Override
  public String getId() {
    return PROVIDER_ID;
  }

  private EventPublisher buildPublisher(Config.Scope config) {
    String brokers = getEnvString(config, "kafka_brokers");
    String securityProvidersConfig = config.get("kafka_security_providers_config");
    String topic = getEnvString(config, "topic");
    long closeTimeout = getEnvLong(config, "close_timeout");
    int maxRetries = getEnvInt(config, "max_retries");
    long initialBackoffMillis = getEnvLong(config, "initial_backoff_millis");

    int deliveryTimeout = getEnvInt(config, "kafka_delivery_timeout_ms_config");
    int requestTimeout = getEnvInt(config, "kafka_request_timeout_ms_config");
    long lingerMs = getEnvLong(config, "kafka_linger_ms");
    ObjectMapper objectMapper = new ObjectMapper();

    Properties props = new Properties();
    props.put(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
    props.put(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG, StringSerializer.class.getName());
    props.put(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG, ByteArraySerializer.class.getName());

    props.put(ProducerConfig.ACKS_CONFIG, "all");
    props.put(ProducerConfig.DELIVERY_TIMEOUT_MS_CONFIG, deliveryTimeout);
    props.put(ProducerConfig.REQUEST_TIMEOUT_MS_CONFIG, requestTimeout);
    props.put(ProducerConfig.LINGER_MS_CONFIG, lingerMs);
    props.put(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, true);
    props.put(ProducerConfig.MAX_IN_FLIGHT_REQUESTS_PER_CONNECTION, 5);

    props.put("security.protocol", securityProvidersConfig);
    String saslMechanism = config.get("kafka_sasl_mechanism");
    if (saslMechanism != null && !saslMechanism.isBlank()) {
      String kafkaUser = getEnvString(config, "kafka_user");
      String kafkaPassword = getEnvString(config, "kafka_password");
      props.put("sasl.mechanism", saslMechanism);
      String jaasConfig =
          String.format(
              "org.apache.kafka.common.security.scram.ScramLoginModule required username=\"%s\""
                  + " password=\"%s\";",
              kafkaUser, kafkaPassword);
      props.put("sasl.jaas.config", jaasConfig);
    }

    KafkaProducer<String, byte[]> producer = new KafkaProducer<>(props);
    return new KafkaEventPublisherImpl(
        producer,
        objectMapper,
        topic,
        deliveryTimeout,
        closeTimeout,
        maxRetries,
        initialBackoffMillis);
  }

  private String getEnvString(Config.Scope config, String key) {
    String env = config.get(key);
    if (env == null || StringUtil.isBlank(env)) {
      throw new RuntimeException(String.format("%s not configured for SPI", key));
    }

    return env;
  }

  private long getEnvLong(Config.Scope config, String key) {
    Long env = config.getLong(key);
    if (env == null) {
      throw new RuntimeException(String.format("%s not configured for SPI", key));
    }

    return env;
  }

  private int getEnvInt(Config.Scope config, String key) {
    Integer env = config.getInt(key);
    if (env == null) {
      throw new RuntimeException(String.format("%s not configured for SPI", key));
    }

    return env;
  }
}
