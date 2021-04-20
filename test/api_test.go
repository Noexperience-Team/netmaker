package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	controller "github.com/gravitl/netmaker/controllers"
	"github.com/gravitl/netmaker/models"
	"github.com/gravitl/netmaker/mongoconn"
	"github.com/stretchr/testify/assert"
)

type databaseError struct {
	Inner  *int
	Errors int
}

//should be use models.SuccessResponse and models.SuccessfulUserLoginResponse
//rather than creating new type but having trouble decoding that way
type Auth struct {
	Username  string
	AuthToken string
}
type Success struct {
	Code     int
	Message  string
	Response Auth
}

type AuthorizeTestCase struct {
	testname      string
	name          string
	password      string
	code          int
	tokenExpected bool
	errMessage    string
}

var Networks []models.Network
var baseURL string = "http://localhost:8081"

func TestMain(m *testing.M) {
	mongoconn.ConnectDatabase()
	var waitgroup sync.WaitGroup
	waitgroup.Add(1)
	go controller.HandleRESTRequests(&waitgroup)
	var gconf models.GlobalConfig
	gconf.ServerGRPC = "localhost:8081"
	gconf.PortGRPC = "50051"
	//err := SetGlobalConfig(gconf)
	collection := mongoconn.Client.Database("netmaker").Collection("config")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	//create, _, err := functions.GetGlobalConfig()
	_, err := collection.InsertOne(ctx, gconf)
	if err != nil {
		panic("could not create config store")
	}

	//wait for http server to start
	time.Sleep(time.Second * 1)
	os.Exit(m.Run())
}

func adminExists(t *testing.T) bool {
	response, err := http.Get("http://localhost:8081/api/users/adm/hasadmin")
	assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	defer response.Body.Close()
	var body bool
	json.NewDecoder(response.Body).Decode(&body)
	return body
}

func api(t *testing.T, data interface{}, method, url, authorization string) (*http.Response, error) {
	var request *http.Request
	var err error
	if data != "" {
		payload, err := json.Marshal(data)
		assert.Nil(t, err, err)
		request, err = http.NewRequest(method, url, bytes.NewBuffer(payload))
		assert.Nil(t, err, err)
		request.Header.Set("Content-Type", "application/json")
	} else {
		request, err = http.NewRequest(method, url, nil)
		assert.Nil(t, err, err)
	}
	if authorization != "" {
		request.Header.Set("authorization", "Bearer "+authorization)
	}
	client := http.Client{}
	//t.Log("api request", request)
	return client.Do(request)
}

func addAdmin(t *testing.T) {
	var admin models.User
	admin.UserName = "admin"
	admin.Password = "password"
	response, err := api(t, admin, http.MethodPost, baseURL+"/api/users/adm/createadmin", "secretkey")
	assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func authenticate(t *testing.T) (string, error) {
	var admin models.User
	admin.UserName = "admin"
	admin.Password = "password"
	response, err := api(t, admin, http.MethodPost, baseURL+"/api/users/adm/authenticate", "secretkey")
	assert.Nil(t, err, err)

	var body Success
	err = json.NewDecoder(response.Body).Decode(&body)
	assert.Nil(t, err, err)
	assert.NotEmpty(t, body.Response.AuthToken, "token not returned")
	assert.Equal(t, "W1R3: Device admin Authorized", body.Message)

	return body.Response.AuthToken, nil
}

func deleteAdmin(t *testing.T) {
	if !adminExists(t) {
		return
	}
	token, err := authenticate(t)
	assert.Nil(t, err, err)
	_, err = api(t, "", http.MethodDelete, baseURL+"/api/users/admin", token)
	assert.Nil(t, err, err)
}

func createNetwork(t *testing.T) {
	network := models.Network{}
	network.NetID = "skynet"
	network.AddressRange = "10.71.0.0/16"
	response, err := api(t, network, http.MethodPost, baseURL+"/api/networks", "secretkey")
	assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func createKey(t *testing.T) {
	key := models.AccessKey{}
	key.Name = "skynet"
	key.Uses = 10
	response, err := api(t, key, http.MethodPost, baseURL+"/api/networks/skynet/keys", "secretkey")
	assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	defer response.Body.Close()
	message, err := ioutil.ReadAll(response.Body)
	assert.Nil(t, err, err)
	assert.NotNil(t, message, message)
}

func getKey(t *testing.T, name string) models.AccessKey {
	response, err := api(t, "", http.MethodGet, baseURL+"/api/networks/skynet/keys", "secretkey")
	assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	defer response.Body.Close()
	var keys []models.AccessKey
	err = json.NewDecoder(response.Body).Decode(&keys)
	assert.Nil(t, err, err)
	for _, key := range keys {
		if key.Name == name {
			return key
		}
	}
	return models.AccessKey{}
}

func deleteKey(t *testing.T, key, network string) {
	response, err := api(t, "", http.MethodDelete, baseURL+"/api/networks/"+network+"/keys/"+key, "secretkey")
	assert.Nil(t, err, err)
	//api does not return Deleted Count at this time
	//defer response.Body.Close()
	//var message mongo.DeleteResult
	//err = json.NewDecoder(response.Body).Decode(&message)
	//assert.Nil(t, err, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	//assert.Equal(t, int64(1), message.DeletedCount)
}

func networkExists(t *testing.T) bool {
	response, err := api(t, "", http.MethodGet, baseURL+"/api/networks", "secretkey")
	assert.Nil(t, err, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	err = json.NewDecoder(response.Body).Decode(&Networks)
	assert.Nil(t, err, err)
	for i, network := range Networks {
		t.Log(i, network)
		if network.NetID == "" {
			return false
		} else {
			return true
		}
	}
	return false
}

func deleteNetworks(t *testing.T) {

	response, err := api(t, "", http.MethodGet, baseURL+"/api/networks", "secretkey")
	assert.Nil(t, err, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	err = json.NewDecoder(response.Body).Decode(&Networks)
	assert.Nil(t, err, err)
	for _, network := range Networks {
		name := network.NetID
		_, err := api(t, "", http.MethodDelete, baseURL+"/api/networks/"+name, "secretkey")
		assert.Nil(t, err, err)
	}
}