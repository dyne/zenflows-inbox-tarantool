package main

import (
	_ "embed"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"bytes"
	"errors"
	"strings"
)

type Config struct {
	port   int
	host   string
	ttHost string
	ttUser string
	ttPass string
	zfUrl  string
}

type Message struct {
	Sender    string                 `json:"sender"`
	Receivers []string               `json:"receivers"`
	Content   map[string]interface{} `json:"content"`
}

type Storage interface {
	send(Message) (int, error)
	read(string, bool) ([]ReadAll, error)
	set(string, int, bool) error
	countUnread(string) (int, error)
	delete(string, int) error

	actorLikes(Activity) (uint64, error)
	findActorLike(uint64) (*Activity, error)
	findActorLikes(string) ([]uint64, error)

	storeFollower(Activity, bool) (bool, uint64, error)
	acceptFollower(uint64) error

	findActorFollows(string, bool) ([]string, error)
}

type Inbox struct {
	storage       Storage
	zfUrl         string
	zenflowsAgent ZenflowsAgent
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, zenflows-sign, zenflows-id")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

//go:embed zenflows-crypto/src/verify_graphql.zen
var VERIFY string

func (inbox *Inbox) sendHandler(c *gin.Context) {
	// Setup json response
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = "Could not read the body of the request"
		return
	}
	zenroomData := ZenroomData{
		Gql:            b64.StdEncoding.EncodeToString(body),
		EdDSASignature: c.Request.Header.Get("zenflows-sign"),
	}

	// Read a message object, I need the receivers
	var message Message
	err = json.Unmarshal(body, &message)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	if len(message.Receivers) == 0 {
		result["error"] = "No receivers"
		return
	}

	if len(message.Content) == 0 {
		result["error"] = "Empty content"
		return
	}
	err = zenroomData.requestPublicKey(inbox.zfUrl, message.Sender)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.isAuth()
	if err != nil {
		result["error"] = err.Error()
		return
	}

	// For each receiver put the message in the inbox
	count, err := inbox.storage.send(message)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	result["success"] = true
	result["count"] = count
	return
}

type ReadMessages struct {
	RequestId  int    `json:"request_id"`
	Receiver   string `json:"receiver"`
	OnlyUnread bool   `json:"only_unread"`
}

func (inbox *Inbox) readHandler(c *gin.Context) {
	// Setup json response
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	// Verify signature request
	zenroomData := ZenroomData{
		Gql:            b64.StdEncoding.EncodeToString(body),
		EdDSASignature: c.Request.Header.Get("zenflows-sign"),
	}
	var readMessage ReadMessages
	err = json.Unmarshal(body, &readMessage)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.requestPublicKey(inbox.zfUrl, readMessage.Receiver)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.isAuth()
	if err != nil {
		result["error"] = err.Error()
		return
	}
	messages, err := inbox.storage.read(readMessage.Receiver, readMessage.OnlyUnread)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	result["success"] = true
	result["request_id"] = readMessage.RequestId
	result["messages"] = messages
	return
}

type SetMessage struct {
	MessageId int    `json:"message_id"`
	Receiver  string `json:"receiver"`
	Read      bool   `json:"read"`
}

func (inbox *Inbox) setHandler(c *gin.Context) {
	// Setup json response
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	// Verify signature request
	zenroomData := ZenroomData{
		Gql:            b64.StdEncoding.EncodeToString(body),
		EdDSASignature: c.Request.Header.Get("zenflows-sign"),
	}
	var setMessage SetMessage
	err = json.Unmarshal(body, &setMessage)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.requestPublicKey(inbox.zfUrl, setMessage.Receiver)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.isAuth()
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = inbox.storage.set(setMessage.Receiver, setMessage.MessageId, setMessage.Read)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	result["success"] = true
	return
}

type CountMessages struct {
	Receiver string `json:"receiver"`
}

func (inbox *Inbox) countHandler(c *gin.Context) {
	// Setup json response
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	// Verify signature request
	zenroomData := ZenroomData{
		Gql:            b64.StdEncoding.EncodeToString(body),
		EdDSASignature: c.Request.Header.Get("zenflows-sign"),
	}
	var countMessages CountMessages
	err = json.Unmarshal(body, &countMessages)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.requestPublicKey(inbox.zfUrl, countMessages.Receiver)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.isAuth()
	if err != nil {
		result["error"] = err.Error()
		return
	}
	count, err := inbox.storage.countUnread(countMessages.Receiver)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	result["success"] = true
	result["count"] = count
	return
}

type DeleteMessage struct {
	MessageId int    `json:"message_id"`
	Receiver  string `json:"receiver"`
}

func (inbox *Inbox) deleteHandler(c *gin.Context) {
	// Setup json response
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	// Verify signature request
	zenroomData := ZenroomData{
		Gql:            b64.StdEncoding.EncodeToString(body),
		EdDSASignature: c.Request.Header.Get("zenflows-sign"),
	}
	var deleteMessage DeleteMessage
	err = json.Unmarshal(body, &deleteMessage)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.requestPublicKey(inbox.zfUrl, deleteMessage.Receiver)
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = zenroomData.isAuth()
	if err != nil {
		result["error"] = err.Error()
		return
	}
	err = inbox.storage.delete(deleteMessage.Receiver, deleteMessage.MessageId)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	result["success"] = true
	return
}

func (inbox *Inbox) profileHandler(actorType string) func(*gin.Context) {
	return func(c *gin.Context) {
		result := map[string]interface{}{
			"success": false,
		}
		defer c.JSON(http.StatusOK, result)
		//actorType := c.Param("type")
		id := c.Param("id")

		baseUrl := fmt.Sprintf("%s/%s/%s", os.Getenv("BASE_URL"), actorType, id)

		var m map[string]interface{} = nil

		switch actorType {
		case "person":
			zfPerson, err := inbox.zenflowsAgent.GetPerson(id)
			if err != nil {
				result["error"] = err.Error()
				return
			}
			m = map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       baseUrl,
				"name":     zfPerson.Name,
				"inbox":    baseUrl + "/inbox",
				"outbox":   baseUrl + "/outbox",
				"type":     "Person",
				"summary":  zfPerson.Note,
			}
		case "economicresource":
			m = map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"id":       baseUrl,
				"name":     "er",
				"summary":  "er",
				//"type":     "EconomicResource",
			}
		default:
			result["success"] = false
			result["error"] = "Unknown actor type: " + actorType
			return
		}

		result["data"] = m
		result["success"] = true
	}
}

type Activity struct {
	Context string `json:"@context"`
	Type    string `json:"type"`
	Id      string `json:"id"`
	Actor   string `json:"actor"`
	Object  string `json:"object"`
	Summary string `json:"summary"`
}

// Takes as input an object like
//
//	{
//		"@context": "https://www.w3.org/ns/activitystreams",
//		"type": "Follow",
//		"actor": `${url}/person/062TE0H7591KJCVT3DDEMDBF0R`,
//		"object": `${url}/person/062TE0YPJD392CS1DPV9XWMDXC`,
//		"published": "2014-09-30T12:34:56Z"
//	}
func (inbox *Inbox) outboxPostHandler(c *gin.Context) {
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	id := c.Param("id")

	var activity Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		result["error"] = err.Error()
		return
	}

	baseUrl := fmt.Sprintf("%s/person/%s", os.Getenv("BASE_URL"), id)
	/*zfPerson, err := inbox.zenflowsAgent.GetPerson(id)
	if err != nil {
		result["error"] = err.Error()
		return
	}*/

	switch activity.Type {
	case "Like":
		cod, err := inbox.storage.actorLikes(activity)
		if err != nil {
			result["error"] = err.Error()
			return
		}

		activity.Id = fmt.Sprintf("%s/liked/%d", baseUrl, cod)

		var jsonmap map[string]interface{}
		tmp, _ := json.Marshal(activity)
		json.Unmarshal(tmp, &jsonmap)

		result["success"] = true
		result["result"] = activity
	case "Follow":
		if _, cod, err := inbox.storage.storeFollower(activity, false); err != nil {
			result["error"] = err.Error()
			return
		} else {
			activity.Id = fmt.Sprintf("%s/follower/%d", activity.Actor, cod)

			tmp, _ := json.Marshal(activity)

			otherInbox := fmt.Sprintf("%s/inbox", activity.Object)
			log.Printf("[APUB] Send follow request to %s\n", otherInbox)

			// TODO: delete stored follow request if the POST fails
			if resp, err := http.Post(otherInbox, "application/json", bytes.NewReader(tmp)); err != nil {
				result["error"] = errors.New("Could not deliver follow request")
				return
			} else if resp.StatusCode != 200 {
				result["error"] = errors.New("Non-200 status when follow request was sent")
				return
			}
			result["data"] = activity
		}

	default:
		result["error"] = "Unknown activity type"
	}
	result["success"] = true

}

func (inbox *Inbox) inboxPostHandler(c *gin.Context) {
	status := http.StatusInternalServerError
	result := map[string]interface{}{
		"success": false,
	}
	defer func() {
		c.JSON(status, result)
	}()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	id := c.Param("id")
	actorType := c.Param("type")

	var activity Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		result["error"] = err.Error()
		return
	}

	baseUrl := fmt.Sprintf("%s/%s/%s", os.Getenv("BASE_URL"), actorType, id)
	/*zfPerson, err := inbox.zenflowsAgent.GetPerson(id)
	if err != nil {
		result["error"] = err.Error()
		return
	}*/

	switch activity.Type {
	case "Follow":
		if _, _, err := inbox.storage.storeFollower(activity, true); err != nil {
			result["error"] = err.Error()
			return
		}
		acceptActivity := &Activity{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Accept",
			Actor:   activity.Object,
			Object:  activity.Id,
		}
		var jsonmap map[string]interface{}
		tmp, _ := json.Marshal(acceptActivity)
		json.Unmarshal(tmp, &jsonmap)

		otherInbox := fmt.Sprintf("%s/inbox", activity.Actor)
		log.Printf("[APUB] Send accept request to %s\n", otherInbox)

		if resp, err := http.Post(otherInbox, "application/json", bytes.NewReader(tmp)); err != nil {
			result["error"] = errors.New(fmt.Sprintf("Could not deliver follow request: %s", err.Error()))
			return
		} else if resp.StatusCode != 200 {
			bodyResp, err := io.ReadAll(c.Request.Body)
			if err != nil {
				result["error"] = err.Error()
				return
			}
			log.Println("Accept error: ", bodyResp)
			result["error"] = errors.New(fmt.Sprintf("Status when follow request was sent: %d", resp.StatusCode))
			return
		}
		result["data"] = acceptActivity
	case "Accept":
		log.Printf("[APUB] I am going to accept %s\n", activity.Object)
		codStr := strings.TrimPrefix(activity.Object, fmt.Sprintf("%s/follower/", baseUrl))
		cod, err := strconv.ParseUint(codStr, 10, 64)
		if err != nil {
			log.Println("[APUB] Exit with error ", err.Error())
			result["error"] = err.Error()
			return
		}
		log.Printf("[APUB] Parsed id %d\n", cod)
		if err := inbox.storage.acceptFollower(cod); err != nil {
			log.Println(err.Error())
			result["error"] = err.Error()
			return
		}
		result["data"] = activity

	default:
		result["error"] = "Unknown activity type"
	}

	log.Println("Inbox finished")
	status = http.StatusOK
	result["success"] = true

}

func (inbox *Inbox) likedHandler(c *gin.Context) {
	result := map[string]interface{}{
		"success": false,
	}
	defer c.JSON(http.StatusOK, result)

	id := c.Param("id")
	actorType := c.Param("type")

	baseUrl := fmt.Sprintf("%s/%s/%s", os.Getenv("BASE_URL"), actorType, id)

	likedIds, err := inbox.storage.findActorLikes(baseUrl)
	if err != nil {
		result["error"] = err.Error()
		return
	}

	items := []string{}
	for i := 0; i < len(likedIds); i = i + 1 {
		likeUrl := fmt.Sprintf("%s/liked/%d", baseUrl, likedIds[i])
		items = append(items, likeUrl)
	}

	jsonmap := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Collection",
		"items":    items,
	}
	result["success"] = true
	result["data"] = jsonmap
}

func (inbox *Inbox) likedIdHandler(actorType string) func(*gin.Context) {
	return func(c *gin.Context) {
		result := map[string]interface{}{
			"success": false,
		}
		defer c.JSON(http.StatusOK, result)

		id := c.Param("id")
		liked := c.Param("liked")

		baseUrl := fmt.Sprintf("%s/person/%s", os.Getenv("BASE_URL"), id)

		var likedId uint64 = 0
		var err error
		if likedId, err = strconv.ParseUint(liked, 10, 64); err != nil {
			result["error"] = err.Error()
			return
		}

		likedActivity, err := inbox.storage.findActorLike(likedId)
		if err != nil {
			result["error"] = err.Error()
			return
		}

		likedActivity.Id = fmt.Sprintf("%s/liked/%d", baseUrl, likedId)

		var jsonmap map[string]interface{}
		tmp, _ := json.Marshal(likedActivity)
		json.Unmarshal(tmp, &jsonmap)

		result["success"] = true
		result["data"] = jsonmap
	}
}

func (inbox *Inbox) followHandler(follower bool) func(c *gin.Context) {
	return func(c *gin.Context) {
		result := map[string]interface{}{
			"success": false,
		}
		defer c.JSON(http.StatusOK, result)

		id := c.Param("id")
		actorType := c.Param("type")

		baseUrl := fmt.Sprintf("%s/%s/%s", os.Getenv("BASE_URL"), actorType, id)

		ids, err := inbox.storage.findActorFollows(baseUrl, follower)
		if err != nil {
			result["error"] = err.Error()
			return
		}

		result["success"] = true
		result["data"] = ids
	}
}

func loadEnvConfig() Config {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	return Config{
		host:   os.Getenv("HOST"),
		port:   port,
		ttHost: os.Getenv("TT_HOST"),
		ttUser: os.Getenv("TT_USER"),
		ttPass: os.Getenv("TT_PASS"),
		zfUrl:  fmt.Sprintf("%s/api", os.Getenv("ZENFLOWS_URL")),
	}
}

func main() {
	config := loadEnvConfig()
	log.Printf("Using backend %s\n", config.zfUrl)

	za := ZenflowsAgent{
		Sk:          os.Getenv("ZENFLOWS_SK"),
		ZenflowsUrl: config.zfUrl,
	}

	storage := &TTStorage{}
	err := storage.Init(config.ttHost, config.ttUser, config.ttPass)
	if err != nil {
		log.Fatal(err.Error())
	}
	inbox := &Inbox{
		storage:       storage,
		zfUrl:         config.zfUrl,
		zenflowsAgent: za,
	}

	r := gin.Default()
	r.SetTrustedProxies(nil)
	r.Use(CORS())

	r.POST("/send", inbox.sendHandler)
	r.POST("/read", inbox.readHandler)
	r.POST("/set-read", inbox.setHandler)
	r.POST("/count-unread", inbox.countHandler)
	r.POST("/delete", inbox.deleteHandler)

	// TODO: why /:type/:id didn't work????
	r.GET("/person/:id", inbox.profileHandler("person"))
	r.GET("/economicresource/:id", inbox.profileHandler("economicresource"))

	r.POST("/:type/:id/inbox", inbox.inboxPostHandler)

	r.POST("/person/:id/outbox", inbox.outboxPostHandler)
	r.GET("/:type/:id/liked", inbox.likedHandler)
	r.GET("/person/:id/liked/:liked", inbox.likedIdHandler("person"))
	r.GET("/economicresource/:id/liked/:liked", inbox.likedIdHandler("economicresource"))

	r.GET("/:type/:id/follower", inbox.followHandler(false))
	r.GET("/:type/:id/following", inbox.followHandler(true))

	host := fmt.Sprintf("%s:%d", config.host, config.port)
	log.Printf("Starting service on %s\n", host)
	r.Run()
}
