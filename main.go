package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type App struct {
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	provider     *oidc.Provider
}

type UserInfo struct {
	Subject        string `json:"sub"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Username       string `json:"preferred_username"`
	GivenName      string `json:"given_name"`
	FamilyName     string `json:"family_name"`
	EmployeeNumber string `json:"employee-number"`
}

// HTML templates
const indexTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>MET.no SSO Hello World</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .login-btn { background: #007acc; color: white; padding: 15px 30px; text-decoration: none; border-radius: 5px; display: inline-block; }
        .login-btn:hover { background: #005a99; }
        .user-info { background: #f5f5f5; padding: 20px; border-radius: 5px; margin: 20px 0; }
        .logout-btn { background: #dc3545; color: white; padding: 10px 20px; text-decoration: none; border-radius: 3px; }
        .logout-btn:hover { background: #c82333; }
    </style>
</head>
<body>
    <h1>MET.no SSO Hello World</h1>
    
    {{if .User}}
        <div class="user-info">
            <h2>Welcome, {{.User.Name}}!</h2>
            <p><strong>Subject:</strong> {{.User.Subject}}</p>
            <p><strong>Email:</strong> {{.User.Email}}</p>
            <p><strong>Username:</strong> {{.User.Username}}</p>
            <p><strong>Given Name:</strong> {{.User.GivenName}}</p>
            <p><strong>Family Name:</strong> {{.User.FamilyName}}</p>
            {{if .User.EmployeeNumber}}<p><strong>Employee Number:</strong> {{.User.EmployeeNumber}}</p>{{end}}
        </div>
        <a href="/logout" class="logout-btn">Logout</a>
    {{else}}
        <p>Welcome to the MET.no SSO Hello World application!</p>
        <p>Please log in to continue.</p>
        <a href="/login" class="login-btn">Login with MET.no</a>
    {{end}}
</body>
</html>
`

func main() {
	// Configuration
	issuerURL := "https://login.met.no/auth/realms/Internal"
	clientID := "modellprod"
	clientSecret := os.Getenv("CLIENT_SECRET")
	if clientSecret == "" {
		log.Printf("Env var CLIENT_SECRET not set. Bailing out")
		os.Exit(1)
	}

	// Detect environment for redirect URL
	redirectURL := "http://localhost:8080/callback"
	if os.Getenv("ENVIRONMENT") == "production" {
		redirectURL = "https://ragdoll.k8s.met.no/rags/callback"
	}

	// Initialize OIDC provider
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		log.Fatal("Failed to get provider: ", err)
	}

	// Configure OAuth2
	oauth2Config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Configure ID token verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	app := &App{
		oauth2Config: oauth2Config,
		verifier:     verifier,
		provider:     provider,
	}

	// Parse template
	tmpl := template.Must(template.New("index").Parse(indexTemplate))

	// Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		app.handleHome(w, r, tmpl)
	})
	http.HandleFunc("/login", app.handleLogin)
	http.HandleFunc("/callback", app.handleCallback)
	http.HandleFunc("/logout", app.handleLogout)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("Redirect URL configured for: %s", redirectURL)
	log.Printf("Open http://localhost:%s in your browser", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}

func (app *App) handleHome(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	// Check for session cookie
	cookie, err := r.Cookie("session")
	var user *UserInfo

	if err == nil {
		// Decode and verify the ID token stored in cookie
		idToken := cookie.Value
		token, err := app.verifier.Verify(context.Background(), idToken)
		if err == nil {
			var claims UserInfo
			if err := token.Claims(&claims); err == nil {
				user = &claims
			}
		}
	}

	data := struct {
		User *UserInfo
	}{
		User: user,
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

func (app *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate state parameter for CSRF protection
	state := generateState()

	// Store state in session (in production, use a proper session store)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // Only secure in HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Redirect to the OAuth2 provider
	url := app.oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (app *App) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Verify state parameter
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "State cookie not found", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "State parameter mismatch", http.StatusBadRequest)
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	token, err := app.oauth2Config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract and verify the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token field in oauth2 token", http.StatusInternalServerError)
		return
	}

	idToken, err := app.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "Failed to verify ID Token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store the ID token in a session cookie (in production, use proper session management)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    rawIDToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Until(idToken.Expiry).Seconds()),
	})

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (app *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// In production, you might also want to call the end_session_endpoint
	// endSessionURL := "https://login.met.no/auth/realms/Internal/protocol/openid-connect/logout"

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
