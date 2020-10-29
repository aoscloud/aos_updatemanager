// SPDX-License-Identifier: Apache-2.0
//
// Copyright 2019 Renesas Inc.
// Copyright 2019 EPAM Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package umserver_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"gitpct.epam.com/epmd-aepr/aos_common/umprotocol"
	"gitpct.epam.com/epmd-aepr/aos_common/wsclient"

	"aos_updatemanager/config"
	"aos_updatemanager/umserver"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const serverURL = "wss://localhost:8088"

/*******************************************************************************
 * Types
 ******************************************************************************/

type testClient struct {
	wsClient       *wsclient.Client
	messageChannel chan []byte
}

type testUpdater struct {
	status        []umprotocol.ComponentStatus
	statusChannel chan []umprotocol.ComponentStatus
}

type testCrtHandler struct {
	csr    string
	crtURL string
	keyURL string
	err    error
}

/*******************************************************************************
 * Vars
 ******************************************************************************/

var updater *testUpdater
var crtHandler *testCrtHandler

/*******************************************************************************
 * Init
 ******************************************************************************/

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000",
		FullTimestamp:    true})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

/*******************************************************************************
 * Main
 ******************************************************************************/

func TestMain(m *testing.M) {
	ret := m.Run()

	time.Sleep(time.Second)

	os.Exit(ret)
}

/*******************************************************************************
 * Tests
 ******************************************************************************/

func TestGetComponents(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	status := []umprotocol.ComponentStatus{
		{ID: "id0", AosVersion: 1, VendorVersion: "1.0", Status: umprotocol.StatusInstalled},
		{ID: "id0", AosVersion: 2, VendorVersion: "2.0", Status: umprotocol.StatusError, Error: "can't install"},
	}

	updater.status = status

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var response []umprotocol.ComponentStatus

	if err := client.sendRequest(umprotocol.GetComponentsRequestType, nil, &response, 5*time.Second); err != nil {
		t.Fatalf("Can't send request: %s", err)
	}

	if !reflect.DeepEqual(response, status) {
		t.Errorf("Wrong updater status: %v %v", status, response)
	}
}

func TestUpdate(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	status := []umprotocol.ComponentStatus{
		{ID: "id0", AosVersion: 2, VendorVersion: "2.0", Status: umprotocol.StatusInstalled},
		{ID: "id1", AosVersion: 3, VendorVersion: "3.0", Status: umprotocol.StatusInstalled},
	}

	var (
		request  []umprotocol.ComponentInfo
		response []umprotocol.ComponentStatus
	)

	for _, item := range status {
		request = append(request, umprotocol.ComponentInfo{
			ID:            item.ID,
			AosVersion:    item.AosVersion,
			VendorVersion: item.VendorVersion,
		})
	}

	if err := client.sendRequest(umprotocol.UpdateRequestType, &request, &response, 5*time.Second); err != nil {
		t.Fatalf("Can't send request: %s", err)
	}

	if !reflect.DeepEqual(response, status) {
		t.Errorf("Wrong updater status: %v %v", status, response)
	}
}

func TestUnsupportedRequest(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var (
		request  []umprotocol.ComponentInfo
		response []umprotocol.ComponentStatus
	)

	if err = client.sendRequest("WrongRequest", &request, &response, 5*time.Second); err == nil {
		t.Fatal("Error is expected here")
	}

	if err = client.sendVersionedRequest(umprotocol.GetComponentsRequestType, &request, &response, 5*time.Second, 255); err == nil {
		t.Fatal("Error is expected here")
	}

	// Test with completely wrong message format
	message := "Some wrong message"

	if err = client.sendRawRequest(&message, &response, 5*time.Second); err == nil {
		t.Fatal("Error is expected here")
	}
}

func TestNilDataRequest(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var response []umprotocol.ComponentStatus

	message := umprotocol.Message{
		Header: umprotocol.Header{
			Version:     umprotocol.Version,
			MessageType: umprotocol.UpdateRequestType,
		},
	}

	message.Data = nil

	if err = client.sendRawRequest(&message, &response, 5*time.Second); err == nil {
		t.Fatal("Expected error here")
	}
}

func TestCreateKeys(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	crtHandler.csr = "this is csr"

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var response umprotocol.CreateKeysRsp
	request := umprotocol.CreateKeysReq{Type: "online"}

	if err := client.sendRequest(umprotocol.CreateKeysRequestType, &request, &response, 5*time.Second); err != nil {
		t.Fatalf("Can't send request: %s", err)
	}

	if response.Type != request.Type {
		t.Errorf("Wrong response type: %s", response.Type)
	}

	if string(response.Csr) != string(crtHandler.csr) {
		t.Errorf("Wrong CSR value: %s", string(response.Csr))
	}

	if response.Error != "" {
		t.Errorf("Response error: %s", response.Error)
	}
}

func TestApplyCert(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	crtHandler.crtURL = "crtURL"

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var response umprotocol.ApplyCertRsp
	request := umprotocol.ApplyCertReq{Type: "online"}

	if err := client.sendRequest(umprotocol.ApplyCertRequestType, &request, &response, 5*time.Second); err != nil {
		t.Fatalf("Can't send request: %s", err)
	}

	if response.Type != request.Type {
		t.Errorf("Wrong response type: %s", response.Type)
	}

	if response.CrtURL != crtHandler.crtURL {
		t.Errorf("Wrong crt URL: %s", response.CrtURL)
	}

	if response.Error != "" {
		t.Errorf("Response error: %s", response.Error)
	}
}

func TestGetCert(t *testing.T) {
	server := newTestServer(serverURL)
	defer server.Close()

	crtHandler.crtURL = "crtURL"
	crtHandler.keyURL = "keyURL"

	client, err := newTestClient(serverURL)
	if err != nil {
		t.Fatalf("Can't create test client: %s", err)
	}
	defer client.close()

	var response umprotocol.GetCertRsp
	request := umprotocol.GetCertReq{Issuer: []byte("issuer"), Serial: "serial"}

	if err := client.sendRequest(umprotocol.GetCertRequestType, &request, &response, 5*time.Second); err != nil {
		t.Fatalf("Can't send request: %s", err)
	}

	if response.Type != request.Type {
		t.Errorf("Wrong response type: %s", response.Type)
	}

	if response.CrtURL != "crtURL" {
		t.Errorf("Wrong crt URL: %s", response.CrtURL)
	}

	if response.KeyURL != "keyURL" {
		t.Errorf("Wrong key URL: %s", response.KeyURL)
	}

	if response.Error != "" {
		t.Errorf("Response error: %s", response.Error)
	}
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func createServerConfig(serverAddress string) (cfg config.Config) {
	configJSON := `{
	"Cert": "../data/crt.pem",
	"Key":  "../data/key.pem"
}`

	decoder := json.NewDecoder(strings.NewReader(configJSON))
	// Parse config
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatalf("Can't parse config: %s", err)
	}

	url, err := url.Parse(serverAddress)
	if err != nil {
		log.Fatalf("Can't parse url: %s", err)
	}

	cfg.ServerURL = url.Host

	return cfg
}

// Success or die
func newTestServer(url string) (server *umserver.Server) {
	cfg := createServerConfig(serverURL)

	updater = &testUpdater{
		statusChannel: make(chan []umprotocol.ComponentStatus)}

	crtHandler = &testCrtHandler{}

	server, err := umserver.New(&cfg, updater, crtHandler)
	if err != nil {
		log.Fatalf("Can't create ws server: %s", err)
	}

	// There is raise condition: after new listen is not started yet
	// so we need this delay to wait for listen
	time.Sleep(time.Second)

	return server
}

func newTestClient(url string) (client *testClient, err error) {
	client = &testClient{messageChannel: make(chan []byte, 1)}

	if client.wsClient, err = wsclient.New("TestClient", client.messageHandler); err != nil {
		return nil, err
	}

	if err = client.wsClient.Connect(url); err != nil {
		return nil, err
	}

	return client, nil
}

func (client *testClient) close() {
	client.wsClient.Close()
}

func (client *testClient) messageHandler(message []byte) {
	client.messageChannel <- message
}

func (client *testClient) sendRequest(messageType string, request, response interface{}, timeout time.Duration) (err error) {
	return client.sendVersionedRequest(messageType, request, response, timeout, umprotocol.Version)
}

func (client *testClient) sendVersionedRequest(messageType string, request, response interface{}, timeout time.Duration, version uint64) (err error) {
	message := umprotocol.Message{
		Header: umprotocol.Header{
			Version:     version,
			MessageType: messageType,
		},
	}

	if request != nil {
		if message.Data, err = json.Marshal(request); err != nil {
			return err
		}
	}

	return client.sendRawRequest(message, response, timeout)
}

func (client *testClient) sendRawRequest(message, response interface{}, timeout time.Duration) (err error) {
	if err = client.wsClient.SendMessage(&message); err != nil {
		return err
	}

	select {
	case <-time.After(timeout):
		return errors.New("wait response timeout")

	case messageJSON := <-client.messageChannel:
		var message umprotocol.Message

		if err = json.Unmarshal(messageJSON, &message); err != nil {
			return err
		}

		if err = json.Unmarshal(message.Data, response); err != nil {
			return err
		}
	}

	return nil
}

func (updater *testUpdater) GetStatus() (status []umprotocol.ComponentStatus) {
	return updater.status
}

func (updater *testUpdater) Update(infos []umprotocol.ComponentInfo) {
	updater.status = nil

	for _, info := range infos {
		updater.status = append(updater.status, umprotocol.ComponentStatus{
			ID:            info.ID,
			AosVersion:    info.AosVersion,
			VendorVersion: info.VendorVersion,
			Status:        umprotocol.StatusInstalled,
		})
	}

	updater.statusChannel <- updater.status
}

func (updater *testUpdater) StatusChannel() (statusChannel <-chan []umprotocol.ComponentStatus) {
	return updater.statusChannel
}

func (handler *testCrtHandler) CreateKeys(crtType, systemID, password string) (csr string, err error) {
	return handler.csr, handler.err
}

func (handler *testCrtHandler) ApplyCertificate(crtType string, crt string) (crtURL string, err error) {
	return handler.crtURL, handler.err
}

func (handler *testCrtHandler) GetCertificate(crtType string, issuer []byte, serial string) (crtURL, keyURL string, err error) {
	return handler.crtURL, handler.keyURL, handler.err
}
