ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak:26.6.0
ARG MAVEN_IMAGE=maven:3.9.14-eclipse-temurin-21-alpine

FROM ${MAVEN_IMAGE} AS jar-builder

WORKDIR /app

# Build user event listener spi
COPY ./keycloak-user-event-listener/pom.xml ./keycloak-user-event-listener/
COPY ./pom.xml .
RUN mvn dependency:go-offline -B

COPY ./keycloak-user-event-listener/src ./keycloak-user-event-listener/src
RUN mvn clean package -DskipTests

FROM ${KEYCLOAK_IMAGE} AS builder

COPY --from=jar-builder /app/keycloak-user-event-listener/target/keycloak-user-event-listener-1.0.0.jar /opt/keycloak/providers/
RUN /opt/keycloak/bin/kc.sh build

FROM ${KEYCLOAK_IMAGE}

COPY --from=builder /opt/keycloak/providers/ /opt/keycloak/providers/

