package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Nom du cookie, du paramètre portant le jeton, et de l'en-tête que seule
// l'interface envoie.
const (
	cookieName    = "cohorte_jeton"
	tokenParam    = "jeton"
	requestHeader = "X-Cohorte"
)

// newToken tire le jeton d'ouverture de session : il figure une seule fois dans
// l'adresse remise à la personne, puis vit dans un cookie.
func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// sameToken compare deux jetons sans fuiter leur contenu par le temps de calcul.
func sameToken(given, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(given), []byte(expected)) == 1
}

// localHost dit si un en-tête Host désigne bien la boucle locale sur le port
// écouté. Un navigateur trompé par une entrée DNS pointant sur 127.0.0.1
// enverrait un autre nom : la requête est alors refusée.
func (s *Server) localHost(host string) bool {
	if host == "" {
		return false
	}
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		// Sans port explicite, l'adresse ne peut pas être la nôtre.
		return false
	}
	if port != s.port {
		return false
	}
	return isLoopback(name)
}

// isLoopback reconnaît « localhost » et les adresses de boucle locale.
func isLoopback(name string) bool {
	name = strings.Trim(strings.ToLower(name), "[]")
	if name == "localhost" {
		return true
	}
	address := net.ParseIP(name)
	return address != nil && address.IsLoopback()
}

// allowedOrigin dit si une origine est celle de l'interface elle-même. Elle est
// exigée sur toute requête qui écrit : sans cela, n'importe quel site ouvert
// dans le navigateur pourrait piloter le serveur.
func (s *Server) allowedOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	return s.localHost(parsed.Host)
}

// guard filtre chaque requête : hôte local, origine attendue, jeton valide.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Rien de ce que sert l'interface ne doit être mis en cache, deviné par
		// son type, intégré dans une autre page, ni fuiter son adresse — jeton
		// compris — par l'en-tête Referer.
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")

		if !s.localHost(request.Host) {
			http.Error(writer, "Hôte inattendu.", http.StatusForbidden)
			return
		}
		// Une origine absente signale une navigation directe ; présente, elle
		// doit être la nôtre, y compris sur les lectures (le corps JSON d'une
		// réponse ne doit jamais atteindre une autre page).
		if origin := request.Header.Get("Origin"); origin != "" && !s.allowedOrigin(origin) {
			http.Error(writer, "Origine refusée.", http.StatusForbidden)
			return
		}
		// Une requête qui écrit doit porter l'en-tête de l'interface : une page
		// d'un autre site ne peut pas l'ajouter sans un contrôle préalable, que
		// ce serveur refuse. Un formulaire tiers en est incapable de toute façon.
		if writes(request.Method) && request.Header.Get(requestHeader) != "1" {
			http.Error(writer, "Requête non reconnue.", http.StatusForbidden)
			return
		}

		// Le jeton passé dans l'adresse ouvre la session, puis disparaît de la
		// barre d'adresse : la suite se joue sur le cookie.
		if token := request.URL.Query().Get(tokenParam); token != "" {
			if !sameToken(token, s.token) {
				http.Error(writer, "Jeton invalide.", http.StatusForbidden)
				return
			}
			http.SetCookie(writer, &http.Cookie{
				Name:     cookieName,
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(writer, request, "/", http.StatusSeeOther)
			return
		}

		cookie, err := request.Cookie(cookieName)
		if err != nil || !sameToken(cookie.Value, s.token) {
			http.Error(writer, "Session absente : rouvrez l'adresse affichée dans le terminal.",
				http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

// writes dit si une méthode modifie l'état du serveur.
func writes(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}
