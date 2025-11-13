ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:26.4.5
ARG MAVEN_IMAGE=maven:3.9.11-eclipse-temurin-21-alpine

FROM ${MAVEN_IMAGE} AS jar-builder

WORKDIR /app/keycloak-user-event-listener

# Build user event listener spi
COPY ./keycloak-user-event-listener/pom.xml .
RUN mvn dependency:go-offline -B

COPY ./keycloak-user-event-listener/src ./src
RUN mvn clean package -DskipTests

FROM ${KEYCLOAK_IMAGE} AS builder

COPY --from=jar-builder /app/keycloak-user-event-listener/target/keycloak-user-event-listener-1.0.0.jar /opt/keycloak/providers/
RUN /opt/keycloak/bin/kc.sh build

FROM ${KEYCLOAK_IMAGE}

COPY --from=builder /opt/keycloak/providers/ /opt/keycloak/providers/
