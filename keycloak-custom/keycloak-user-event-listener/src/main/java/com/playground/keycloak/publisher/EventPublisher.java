package com.playground.keycloak.publisher;

import com.playground.keycloak.dto.EventMessage;

public interface EventPublisher {
  void publish(EventMessage msg);
}
