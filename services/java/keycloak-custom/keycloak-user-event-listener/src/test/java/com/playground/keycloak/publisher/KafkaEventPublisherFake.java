package com.playground.keycloak.publisher;

import com.playground.keycloak.dto.EventMessage;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.atomic.AtomicBoolean;

public class KafkaEventPublisherFake implements EventPublisher {

  private final List<EventMessage> publishedMessages = new ArrayList<>();
  private final AtomicBoolean closed = new AtomicBoolean(false);

  @Override
  public void publish(EventMessage message) {
    // Mimic real implementation: if closed or message is null, it effectively drops the message
    // (real impl logs/retries then drops).
    if (closed.get() || message == null) {
      return;
    }

    synchronized (publishedMessages) {
      publishedMessages.add(message);
    }
  }

  @Override
  public void close() {
    closed.set(true);
  }

  public List<EventMessage> getPublishedMessages() {
    synchronized (publishedMessages) {
      return Collections.unmodifiableList(new ArrayList<>(publishedMessages));
    }
  }

  public EventMessage getLastMessage() {
    synchronized (publishedMessages) {
      if (publishedMessages.isEmpty()) {
        return null;
      }
      return publishedMessages.get(publishedMessages.size() - 1);
    }
  }

  public EventMessage getMessage(int index) {
    synchronized (publishedMessages) {
      return publishedMessages.get(index);
    }
  }

  public boolean isClosed() {
    return closed.get();
  }

  public void clear() {
    synchronized (publishedMessages) {
      publishedMessages.clear();
    }
  }
}
