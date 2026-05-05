# Multi-stage build for Java services
FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app

# Copy pom.xml
COPY services/java/pom.xml .

# Download dependencies
RUN mvn dependency:resolve

# Copy source
COPY services/java .

# Build the application (ARG to specify which module)
ARG JAVA_MODULE=reporting-service
RUN mvn clean package -DskipTests -pl ${JAVA_MODULE} -am

# Runtime stage
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# Copy JAR from builder
ARG JAVA_MODULE=reporting-service
COPY --from=builder /app/${JAVA_MODULE}/target/*.jar app.jar

EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
