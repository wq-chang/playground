package com.playground.keycloak.publisher;

import com.playground.keycloak.dto.EventMessage;

public interface EventPublisher extends AutoCloseable {
  void publish(EventMessage msg);
}
