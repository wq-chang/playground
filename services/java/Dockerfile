# Multi-stage build for Java services
FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app

# Copy pom files
COPY services/java/pom.xml .

# Download dependencies
RUN mvn dependency:resolve

# Copy source
COPY services/java .

# Build all Java modules
RUN mvn clean package -DskipTests

# Runtime stage - uses first built JAR
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# Copy JAR from builder (can be customized per deployment)
COPY --from=builder /app/target/*.jar app.jar

EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
