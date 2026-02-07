package handlers

import (
	"fmt"
	"net/http"
)

func (h *Handler) OAuthSuccessPage(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>OAuth Success</title>
			<style>
				body {
					font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
					display: flex;
					justify-content: center;
					align-items: center;
					height: 100vh;
					margin: 0;
					background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
				}
				.container {
					background: white;
					padding: 40px;
					border-radius: 12px;
					box-shadow: 0 10px 40px rgba(0,0,0,0.1);
					text-align: center;
					max-width: 400px;
				}
				h1 { color: #2d3748; margin-bottom: 10px; }
				.success-icon {
					font-size: 64px;
					margin-bottom: 20px;
				}
				p { color: #718096; }
			</style>
		</head>
		<body>
			<div class="container">
				<div class="success-icon">✅</div>
				<h1>Successfully Connected!</h1>
				<p>Your %s account has been connected.</p>
				<p style="font-size: 14px; margin-top: 20px;">You can close this window now.</p>
			</div>
			<script>
				if (window.opener) {
					window.opener.postMessage({type: 'oauth_success', platform: '%s'}, '*');
					setTimeout(() => window.close(), 3000);
				}
			</script>
		</body>
		</html>
	`, platform, platform)))
}

func (h *Handler) OAuthErrorPage(w http.ResponseWriter, r *http.Request) {
	errorType := r.URL.Query().Get("error")
	description := r.URL.Query().Get("description")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>OAuth Error</title>
			<style>
				body {
					font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
					display: flex;
					justify-content: center;
					align-items: center;
					height: 100vh;
					margin: 0;
					background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%);
				}
				.container {
					background: white;
					padding: 40px;
					border-radius: 12px;
					box-shadow: 0 10px 40px rgba(0,0,0,0.1);
					text-align: center;
					max-width: 400px;
				}
				h1 { color: #e53e3e; margin-bottom: 10px; }
				.error-icon {
					font-size: 64px;
					margin-bottom: 20px;
				}
				p { color: #718096; }
				.error-details {
					background: #fed7d7;
					padding: 15px;
					border-radius: 6px;
					margin-top: 20px;
					font-size: 14px;
					color: #c53030;
				}
			</style>
		</head>
		<body>
			<div class="container">
				<div class="error-icon">❌</div>
				<h1>Connection Failed</h1>
				<p>There was a problem connecting your account.</p>
				<div class="error-details">
					<strong>Error:</strong> %s<br>
					<strong>Details:</strong> %s
				</div>
				<p style="font-size: 14px; margin-top: 20px;">Please try again or contact support.</p>
			</div>
			<script>
				setTimeout(() => window.close(), 5000);
			</script>
		</body>
		</html>
	`, errorType, description)))
}
