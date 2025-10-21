package com.playground.keycloak.listener;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.playground.keycloak.publisher.EventPublisher;
import com.playground.keycloak.publisher.EventPublisherImpl;
import com.playground.keycloak.service.UserEventServiceImpl;
import com.playground.keycloak.util.EventLogger;
import io.nats.client.Connection;
import io.nats.client.JetStream;
import io.nats.client.Nats;
import java.io.IOException;
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
  private Connection natsConnection;

  @Override
  public void init(Config.Scope config) {
    String natsUrl = config.get("nats_url");
    if (natsUrl == null || StringUtil.isBlank(natsUrl)) {
      throw new RuntimeException("NATS URL not configured for SPI");
    }

    try {
      natsConnection = Nats.connect(natsUrl);

      logger.infof("Connected to NATS at %s", natsUrl);
    } catch (Exception e) {
      logger.error("Failed to connect to NATS", e);
      throw new RuntimeException("NATS connection failed — aborting Keycloak startup", e);
    }
    logger.info("Initializing UserEventListener");
  }

  @Override
  public EventListenerProvider create(KeycloakSession session) {
    logger.debug("Creating UserEventListener instance");
    try {
      JetStream js = natsConnection.jetStream();
      ObjectMapper objectMapper = new ObjectMapper();
      EventPublisher publisher = new EventPublisherImpl(js, objectMapper);
      EventLogger eventLogger = new EventLogger();
      UserEventServiceImpl service = new UserEventServiceImpl(eventLogger, publisher);

      return new UserEventListener(service);
    } catch (IOException e) {
      logger.error("Failed to initialize JetStream", e);
      throw new RuntimeException("JetStream Initialization failed - aborting Keycloak startup", e);
    }
  }

  @Override
  public void postInit(KeycloakSessionFactory factory) {
    logger.info("Post-initializing UserEventListener");
  }

  @Override
  public void close() {
    logger.info("Closing UserEventListenerFactory");
    if (natsConnection != null) {
      try {
        natsConnection.close();
      } catch (InterruptedException e) {
        logger.error("Failed to close nats connection", e);
      }
    }
  }

  @Override
  public String getId() {
    return PROVIDER_ID;
  }
}
