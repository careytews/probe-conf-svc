package main

import (
	// Bolt is a simple key-value store.
	"github.com/boltdb/bolt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// State information.
type Handler struct {

	// Key-value store.
	db *bolt.DB
}

// HTTP request handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if strings.HasPrefix(r.URL.Path, "/probe-creds/") {
		h.ServeProbeCreds(w, r,
			strings.TrimPrefix(r.URL.Path, "/probe-creds/"))
		return
	}

	if strings.HasPrefix(r.URL.Path, "/vpn-service-creds/") {
		h.ServeVpnServiceCreds(w, r,
			strings.TrimPrefix(r.URL.Path, "/vpn-service-creds/"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, "Not found.")
	return

}

func main() {

	handler := &Handler{}

	// Open database.
	var err error
	handler.db, err = bolt.Open("data/creds.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create buckets.
	handler.db.Update(func(tx *bolt.Tx) error {

		// Create bucket
		_, err := tx.CreateBucketIfNotExists([]byte(ProbeCredsBucket))
		if err != nil {
			log.Fatal(err)
		}

		// Create bucket
		_, err = tx.CreateBucketIfNotExists([]byte(VpnServiceCredsBucket))
		if err != nil {
			log.Fatal(err)
		}

		return nil
	})

	go handler.resyncProbeCreds()
	go handler.resyncVpnServiceCreds()

	// Start HTTPS server - key/cert supplied.
	s := &http.Server{
		Addr:           ":443",
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServeTLS("creds/cert.server", "creds/key.server"))

}
