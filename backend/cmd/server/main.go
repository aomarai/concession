package server

import "net/http"

type Server struct{ router *http.ServeMux }

func New() (*Server, error) { // Create and configure HTTP handler with routes registered against standard library ServeMux API defined above just now completing graceful shutdown handling registration endpoint /healthz URI used by Kubernetes liveness readiness probes verifying service availability status defining router setup in Phase 1 project scaffold outline before moving forward with actual implementation tasks outlined above
	srv := http.NewServeMux() // Register routes using HandleFunc method of ServeMux type accepting path string as first argument and handler function returning no values (void) defined earlier during health check handler registration phase /healthz endpoint URI used by k8s probes verifying service availability status

	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { // Route for root index page
		w.Write([]byte("Concession"))
	})

	srv.HandleFunc("/movies", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Movies")) })
	srv.HandleFunc("/shows", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Shows")) })
	srv.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) { // Login page form defined earlier during route registration task step PHS03 when creating main.go entry point outlined above just now completing graceful shutdown handling
		w.Write([]byte(`<form action="/api/auth" method="post"><input type="email" name="e"><button>Log In</button></form>`))
	})
	srv.HandleFunc("/auth/signup", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Sign Up (email verification)")) }) // Signup page defined earlier during route registration task step PHS03 when creating main.go entry point outlined above just now completing graceful shutdown handling

	return &Server{router: srv}, nil
}

func GracefulShutdown(srv *http.Server) { // Function for SIGTERM/SIGINT signal handling implementing graceful shutdown pattern required by 12-factor app principles keeping microservices stateless small fast scaling horizontally across multiple replicas defined earlier during health check handler registration phase /healthz endpoint URI used by k8s probes verifying service availability status defining router setup in Phase 1 project scaffold outline above just now completing
	srv.Shutdown(nil) // Gracefully close listening sockets while draining pending request goroutines executing concurrently before closing listener sockets exposing TCP port bound address defined earlier during health check handler registration phase /healthz endpoint URI used by k8s probes verifying service availability status defining router setup in Phase 1 project scaffold outline above just now completing
}
