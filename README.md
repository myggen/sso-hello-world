MET.no SSO Hello World Application
A simple Go web application demonstrating OpenID Connect (OIDC) integration with MET.no's authentication system.

Features
OpenID Connect authentication with login.met.no
Session management with HTTP cookies
CSRF protection with state parameters
Environment-aware redirect URLs (localhost vs production)
User information display from ID token claims
Responsive web interface
Prerequisites
Go 1.21 or later
Access to MET.no's internal authentication system
Local Development Setup
Clone and setup the project:
bash
mkdir sso-hello-world
cd sso-hello-world
Create the files:
Copy the main.go and go.mod files to your project directory
Install dependencies:
bash
go mod tidy
Set environment variables (optional):
bash
export CLIENT_SECRET="your-actual-client-secret"
export PORT="8080"
Run the application:
bash
go run main.go
Open your browser: Navigate to http://localhost:8080
Configuration
Client Registration
The application is configured for:

Client ID: modellprod
Client Secret: placeholder-password-xyz (set via environment variable in production)
Issuer: https://login.met.no/auth/realms/Internal
Redirect URLs
Development: http://localhost:8080/callback
Production: https://ragdoll.k8s.met.no/rags/callback
Make sure these redirect URLs are registered in your OAuth2 client configuration.

Deployment to Kubernetes
Docker Setup
Create a Dockerfile:

dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
Kubernetes Deployment
Create a deployment.yaml:

yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sso-hello-world
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: sso-hello-world
  template:
    metadata:
      labels:
        app: sso-hello-world
    spec:
      containers:
      - name: sso-hello-world
        image: your-registry/sso-hello-world:latest
        ports:
        - containerPort: 8080
        env:
        - name: ENVIRONMENT
          value: "production"
        - name: CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: sso-secrets
              key: client-secret
        - name: PORT
          value: "8080"
---
apiVersion: v1
kind: Service
metadata:
  name: sso-hello-world-service
spec:
  selector:
    app: sso-hello-world
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sso-hello-world-ingress
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - ragdoll.k8s.met.no
    secretName: ragdoll-tls
  rules:
  - host: ragdoll.k8s.met.no
    http:
      paths:
      - path: /rags
        pathType: Prefix
        backend:
          service:
            name: sso-hello-world-service
            port:
              number: 80
Create Kubernetes Secret
bash
kubectl create secret generic sso-secrets \
  --from-literal=client-secret="your-actual-client-secret"
Deploy
bash
kubectl apply -f deployment.yaml
Security Considerations
For Production
Use proper session management: Replace cookie-based sessions with a proper session store (Redis, database)
Secure cookies: Ensure Secure flag is set for HTTPS
Environment variables: Store sensitive data in Kubernetes secrets
HTTPS only: Ensure all communication uses HTTPS in production
Logout endpoint: Consider calling the provider's end_session_endpoint for proper logout
Current Implementation Notes
Uses HTTP cookies for session storage (suitable for demo, not production at scale)
State parameter provides CSRF protection
ID token verification ensures token authenticity
HttpOnly cookies prevent XSS attacks
API Endpoints
GET / - Home page (shows login button or user info)
GET /login - Initiates OAuth2 flow
GET /callback - OAuth2 callback handler
GET /logout - Clears session and logs out
User Information Retrieved
The application retrieves and displays:

Subject (unique user ID)
Name
Email
Username (preferred_username)
Given Name
Family Name
Employee Number (if available)
Troubleshooting
Common Issues
"State parameter mismatch"
Ensure cookies are enabled
Check that redirect URL matches exactly
"Failed to exchange token"
Verify client ID and secret are correct
Check that redirect URL is registered with the OAuth provider
"No id_token field in oauth2 token"
Ensure the openid scope is included in the request
Connection issues to login.met.no
Verify network access to MET.no's internal systems
Check firewall rules if running in restricted environments
Debug Mode
Add debug logging by setting the log level:

go
log.SetLevel(log.DebugLevel)
License
This is a demonstration application. Use according to your organization's policies.


