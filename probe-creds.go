package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	creds "github.com/trustnetworks/credentials"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ProbeCredsBucket = "probe-creds"
)

// Structure for JSON messagesreturns from credential manager via pubsub.
type ProbeCredResponse struct {

	// P12 bundle
	P12 string `json:"p12"`

	// Password
	Password string `json:"password"`

	// Host
	Host string `json:"host"`

	// Whether the request was successful.
	Port int `json:"port"`
}

func (h *Handler) ServeProbeCreds(w http.ResponseWriter, r *http.Request,
	name string) {

	var p12 []byte = nil
	var password []byte = nil
	var host []byte = nil
	var port []byte = nil

	// Fetch stuff from database.
	err := h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ProbeCredsBucket))
		p12 = b.Get([]byte(name + ".p12"))
		password = b.Get([]byte(name + ".password"))
		host = b.Get([]byte(name + ".host"))
		port = b.Get([]byte(name + ".port"))
		return nil
	})

	// Handle failure with a 500 status.
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Database lookup failed.")
		return
	}

	if p12 == nil || password == nil || host == nil || port == nil {
		// Delay to make cred stuff harder.  Ish.
		time.Sleep(time.Second * 3)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not found.")
		return
	}

	portnum, _ := strconv.Atoi(string(port))

	resp := &ProbeCredResponse{
		P12:      base64.StdEncoding.EncodeToString(p12),
		Password: string(password),
		Host:     string(host),
		Port:     portnum,
	}

	respenc, _ := json.Marshal(resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respenc)
	return

}

func (h *Handler) WriteProbeCredToDb(name string, p12, password []byte,
	host, port, end string) error {

	err := h.db.Update(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(ProbeCredsBucket))

		key := name + ".p12"
		err := b.Put([]byte(key), p12)
		if err != nil {
			fmt.Println(err)
		}

		key = name + ".password"
		err = b.Put([]byte(key), password)
		if err != nil {
			fmt.Println(err)
		}

		key = name + ".host"
		err = b.Put([]byte(key), []byte(host))
		if err != nil {
			fmt.Println(err)
		}

		key = name + ".port"
		err = b.Put([]byte(key), []byte(port))
		if err != nil {
			fmt.Println(err)
		}

		key = name + ".end"
		err = b.Put([]byte(key), []byte(end))
		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("Probe", name, "added.")

		return nil

	})

	return err

}

func (h *Handler) DeleteProbeCredToDb(name string) error {

	err := h.db.Update(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(ProbeCredsBucket))

		key := name + ".p12"
		err := b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".password"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".host"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".port"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".end"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		return nil

	})

	return err

}

func (h *Handler) resyncProbeCreds() {

	client, err := creds.NewSaClient("keys/private.json")
	if err != nil {
		fmt.Println("Sync error:", err)
		os.Exit(1)
	}

	user, err := client.GetEmailAddress()
	if err != nil {
		fmt.Println("Sync error:", err)
		os.Exit(1)
	}
	client.SetUser(user)
	fmt.Println("Logged in as", user)

	// Maps cred ID to end time - we'll use that to detect cred change.
	credIds := map[string]string{}

	// Initialise the credIds map from the database cache.
	h.db.Update(func(tx *bolt.Tx) error {

		// Create bucket
		b, err := tx.CreateBucketIfNotExists([]byte(ProbeCredsBucket))
		if err != nil {
			fmt.Println(err)
		}

		// Cursor on all keys.
		c := b.Cursor()

		// Loop through all keys.
		for k, v := c.First(); k != nil; k, v = c.Next() {

			key := string(k)
			if strings.HasSuffix(key, ".end") {
				key = strings.TrimSuffix(key, ".end")
				credIds[key] = string(v)
				fmt.Printf("%s is cached.\n", key)
			}

		}

		return nil
	})

	var indexVersion int64 = -1

	for {

		curVersion, err := client.GetIndexVersion(user)
		if err != nil {
			fmt.Println("Sync error (GetIndexVersion):", err)
			time.Sleep(time.Second * 5)
			continue
		}

		if curVersion == indexVersion {
			time.Sleep(time.Second * 5)
			continue
		}

		fmt.Println("Index updated.")

		crs, err := client.GetIndex(user)
		if err != nil {
			fmt.Println("Sync error (GetIndex):", err)
			time.Sleep(time.Second * 5)
			continue
		}

		newIds := map[string]string{}

		for _, cr := range crs {

			if cr.GetType() != "probe" {
				continue
			}

			pr := cr.(*creds.ProbeCredential)

			name := pr.Name
			end := pr.GetEnd()

			if _, ok := credIds[name]; ok {
				if credIds[name] == end {
					newIds[name] = credIds[name]
					continue
				}
			}

			ps, err := cr.Get(client, "p12")
			if err != nil {
				fmt.Println("Payload error:", err)
				continue
			}

			var p12 []byte
			var password []byte
			for _, p := range ps {
				if p.Id == "p12" {
					p12 = p.Payload
				}
				if p.Id == "password" {
					password = p.Payload
				}
			}

			err = h.WriteProbeCredToDb(name, p12, password, pr.Host, pr.Port, end)
			if err != nil {
				fmt.Println(err)
			}

			newIds[name] = end

		}

		for k, _ := range credIds {
			if _, ok := newIds[k]; !ok {

				err = h.DeleteProbeCredToDb(k)
				if err != nil {
					fmt.Println(err)
				} else {
					fmt.Println(k, "deleted.")
				}

			}
		}

		credIds = newIds
		indexVersion = curVersion

		// Update from index every minute.
		// FIXME: Would be good to have a 'has changed' test
		// on the index file.
		time.Sleep(time.Second * 5)

	}

}
