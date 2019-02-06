package main

import (
	"fmt"
	// Bolt is a simple key-value store.
	"encoding/base64"
	"encoding/json"
	"github.com/boltdb/bolt"
	creds "github.com/trustnetworks/credentials"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	VpnServiceCredsBucket = "vpn-service-creds"
)

// Structure for JSON messagesreturns from credential manager via pubsub.
type VpnServiceCredResponse struct {

	// P12 bundle
	P12 string `json:"p12"`

	// Password
	Password string `json:"password"`

	// Two other paylodas
	Dh string `json:"dh"`
	Ta string `json:"ta"`

	// Host
	Host string `json:"host"`

	Allocator string `json:"allocator"`
	ProbeKey string `json:"probekey"`
}

func (h *Handler) ServeVpnServiceCreds(w http.ResponseWriter, r *http.Request,
	name string) {

	var p12 []byte = nil
	var password []byte = nil
	var host []byte = nil
	var allocator []byte = nil
	var dh []byte = nil
	var ta []byte = nil
	var probekey []byte = nil

	// Fetch stuff from database.
	err := h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(VpnServiceCredsBucket))
		p12 = b.Get([]byte(name + ".p12"))
		password = b.Get([]byte(name + ".password"))
		host = b.Get([]byte(name + ".host"))
		allocator = b.Get([]byte(name + ".allocator"))
		probekey = b.Get([]byte(name + ".probekey"))
		dh = b.Get([]byte(name + ".dh"))
		ta = b.Get([]byte(name + ".ta"))
		return nil
	})

	// Handle failure with a 500 status.
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Database lookup failed.")
		return
	}

	if p12 == nil || password == nil || host == nil || allocator == nil ||
		dh == nil || ta == nil || probekey == nil {
		// Delay to make cred stuff harder.  Ish.
		time.Sleep(time.Second * 3)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not found.")
		return
	}

	resp := &VpnServiceCredResponse{
		P12:           base64.StdEncoding.EncodeToString(p12),
		Password:      string(password),
		Host:          string(host),
		Allocator:     string(allocator),
		ProbeKey:      string(probekey),
		Dh:            base64.StdEncoding.EncodeToString(dh),
		Ta:            base64.StdEncoding.EncodeToString(ta),
	}

	respenc, _ := json.Marshal(resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respenc)
	return

}

func (h *Handler) WriteVpnServiceCredToDb(name string, p12, password []byte,
	host string, allocator, dh, ta []byte, end string,
	probekey []byte) error {

	err := h.db.Update(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(VpnServiceCredsBucket))

		key := name + ".p12"
		err := b.Put([]byte(key), p12)
		if err != nil {
			return err
		}

		key = name + ".password"
		err = b.Put([]byte(key), password)
		if err != nil {
			return err
		}

		key = name + ".host"
		err = b.Put([]byte(key), []byte(host))
		if err != nil {
			return err
		}

		key = name + ".allocator"
		err = b.Put([]byte(key), []byte(allocator))
		if err != nil {
			return err
		}

		key = name + ".dh"
		err = b.Put([]byte(key), []byte(dh))
		if err != nil {
			return err
		}

		key = name + ".ta"
		err = b.Put([]byte(key), []byte(ta))
		if err != nil {
			return err
		}

		key = name + ".end"
		err = b.Put([]byte(key), []byte(end))
		if err != nil {
			return err
		}

		key = name + ".probekey"
		err = b.Put([]byte(key), []byte(probekey))
		if err != nil {
			return err
		}

		fmt.Println("VPN service", name, "added.")

		return nil

	})

	return err

}

func (h *Handler) DeleteVpnServiceCredToDb(name string) error {

	err := h.db.Update(func(tx *bolt.Tx) error {

		b := tx.Bucket([]byte(VpnServiceCredsBucket))

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

		key = name + ".allocator"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".dh"
		err = b.Delete([]byte(key))
		if err != nil {
			return err
		}

		key = name + ".ta"
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

func (h *Handler) resyncVpnServiceCreds() {

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
		b, err := tx.CreateBucketIfNotExists([]byte(VpnServiceCredsBucket))
		if err != nil {
			log.Fatal(err)
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

			if cr.GetType() != "vpn-service" {
				continue
			}

			pr := cr.(*creds.VpnServiceCredential)

			name := pr.Name
			end := pr.GetEnd()

			if _, ok := credIds[name]; ok {
				if credIds[name] == end {
					newIds[name] = credIds[name]
					continue
				}
			}

			host := pr.Host

			ps, err := cr.Get(client, "p12")
			if err != nil {
				fmt.Println("Payload error:", err)
				continue
			}

			var p12 []byte
			var password []byte
			var allocator []byte
			var probekey []byte
			var dh []byte
			var ta []byte

			for _, p := range ps {
				if p.Id == "p12" {
					p12 = p.Payload
				}
				if p.Id == "password" {
					password = p.Payload
				}
				if p.Id == "allocator" {
					allocator = p.Payload
				}
				if p.Id == "probekey" {
					probekey = p.Payload
				}
				if p.Id == "dh.server" {
					dh = p.Payload
				}
				if p.Id == "ta.key" {
					ta = p.Payload
				}
			}

			err = h.WriteVpnServiceCredToDb(name, p12, password,
				host, allocator, dh, ta, end, probekey)
			if err != nil {
				fmt.Println(err)
			}

			newIds[name] = end

		}

		for k, _ := range credIds {

			if _, ok := newIds[k]; !ok {

				err = h.DeleteVpnServiceCredToDb(k)
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
