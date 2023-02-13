package main

import (
	"encoding/json"
	"errors"
	"github.com/tarantool/go-tarantool"
	"log"
	"time"
)

type TTStorage struct {
	db *tarantool.Connection
}

type ReadAll struct {
	Id      int                    `json:"id"`
	Sender  string                 `json:"sender"`
	Content map[string]interface{} `json:"content"`
	Read    bool                   `json:"read"`
}

const MAX_RETRY int = 10

func (storage *TTStorage) Init(host, user, pass string) error {
	var err error
	for done, retry := false, 0; !done; retry++ {
		storage.db, err = tarantool.Connect(host, tarantool.Opts{
			User: user,
			Pass: pass,
		})
		done = retry == MAX_RETRY || err == nil
		if !done {
			log.Println("Could not connect to tarantool, retrying...")
			time.Sleep(3 * time.Second)
		} else {
			log.Println("Connected to tarantool")
		}
	}
	return err
}

func (storage *TTStorage) send(message Message) (int, error) {
	jsonData, err := json.Marshal(message.Content)
	resp, err := storage.db.Insert("messages", []interface{}{nil, string(jsonData), message.Sender})
	if err != nil {
		return 0, err
	}
	message_id := resp.Data[0].([]interface{})[0]
	count := 0
	for i := 0; i < len(message.Receivers); i++ {
		_, err := storage.db.Insert("receivers", []interface{}{message_id, message.Receivers[i], false})
		if err == nil {
			count = count + 1
		}
	}
	return count, nil
}

func (storage *TTStorage) read(who string, onlyUnread bool) ([]ReadAll, error) {
	var filter []interface{}
	if onlyUnread {
		filter = []interface{}{who, false}
	} else {
		filter = []interface{}{who}
	}
	resp, err := storage.db.Select("receivers", "receivers_idx", 0, 4096, tarantool.IterEq, filter)
	messages := make([]ReadAll, 0, 5)
	if err != nil {
		return messages, err
	}
	for _, d := range resp.Data {
		id := d.([]interface{})[0]
		resp2, err := storage.db.Select("messages", "primary", 0, 4096, tarantool.IterEq, []interface{}{id})
		dataRead := resp2.Data[0].([]interface{})

		// read flag could be null
		var read bool
		if len(d.([]interface{})) >= 3 {
			read = d.([]interface{})[2].(bool)
		} else {
			read = false
		}
		current := ReadAll{
			Id:     int(dataRead[0].(uint64)),
			Sender: dataRead[2].(string),
			Read:   read,
		}
		err = json.Unmarshal([]byte(dataRead[1].(string)), &current.Content)
		if err != nil {
			return messages, err
		}
		messages = append(messages, current)
	}
	return messages, nil
}

func (storage *TTStorage) set(who string, message_id int, read bool) error {
	_, err := storage.db.Update("receivers", "primary", []interface{}{uint64(message_id), who}, []interface{}{[]interface{}{"=", 2, read}})
	if err != nil {
		return err
	}
	return nil
}

const LIMIT_MSG = 1000

func (storage *TTStorage) countUnread(who string) (int, error) {
	resp, err := storage.db.Select("receivers", "receivers_idx", 0, LIMIT_MSG, tarantool.IterEq, []interface{}{who, false})
	if err != nil {
		return 0, err
	}

	return len(resp.Data), nil
}

func (storage *TTStorage) delete(who string, message_id int) error {
	_, err := storage.db.Delete("receivers", "primary", []interface{}{uint64(message_id), who})
	if err != nil {
		return err
	}
	return nil
}

func (storage *TTStorage) actorLikes(activity Activity) (uint64, error) {
	if activity.Type != "Like" {
		return 0, errors.New("Not a Like activity")
	}
	resp, err := storage.db.Insert("liked", []interface{}{nil, activity.Actor, activity.Object, activity.Summary})
	if err != nil {
		return 0, err
	} else if resp.Error != "" {
		return 0, errors.New(resp.Error)
	}
	dataWritten := resp.Data[0].([]interface{})

	return dataWritten[0].(uint64), nil
}

func (storage *TTStorage) findActorLike(id uint64) (*Activity, error) {
	resp, err := storage.db.Select("liked", "primary", 0, 1, tarantool.IterEq, []interface{}{id})
	if err != nil {
		return nil, err
	} else if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	data := resp.Data[0].([]interface{})
	act := &Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		Type:    "Like",
		Actor:   data[1].(string),
		Object:  data[2].(string),
		Summary: data[3].(string),
	}
	return act, nil
}

func (storage *TTStorage) findActorLikes(id string) ([]uint64, error) {
	resp, err := storage.db.Select("liked", "actors", 0, LIMIT_MSG, tarantool.IterEq, []interface{}{id})
	if err != nil {
		return nil, err
	}
	var ids []uint64
	for _, d := range resp.Data {
		id := d.([]interface{})[0].(uint64)
		ids = append(ids, id)
	}
	return ids, nil
}

func (storage *TTStorage) storeFollower(activity Activity, accepted bool) (bool, uint64, error) {
	created := false
	if activity.Type != "Follow" {
		return false, 0, errors.New("Not a Follow activity")
	}
	respRead, err := storage.db.Select("follow", "following", 0, LIMIT_MSG, tarantool.IterEq, []interface{}{activity.Object, activity.Actor})
	if err != nil {
		return false, 0, err
	} else if respRead.Error != "" {
		return false, 0, errors.New(respRead.Error)
	}
	data := respRead.Data
	var cod uint64
	if len(data) == 0 {
		resp, err := storage.db.Insert("follow",
			[]interface{}{nil, activity.Actor, activity.Object, accepted})
		if err != nil {
			return false, 0, err
		} else if resp.Error != "" {
			return false, 0, errors.New(resp.Error)
		}
		dataWritten := resp.Data[0].([]interface{})
		cod = dataWritten[0].(uint64)
		created = true
	} else {
		cod = data[0].([]interface{})[0].(uint64)
		currentAccepted := data[0].([]interface{})[3].(bool)
		if !currentAccepted && accepted {
			resp, err := storage.db.Update("follow", "primary",
				[]interface{}{cod},
				[]interface{}{[]interface{}{"=", 4, accepted}})
			if err != nil {
				return false, 0, err
			} else if resp.Error != "" {
				return false, 0, errors.New(resp.Error)
			}
		}
	}

	return created, cod, nil
}

func (storage *TTStorage) acceptFollower(id uint64) error {
	resp, err := storage.db.Update("follow", "primary",
		[]interface{}{id},
		[]interface{}{[]interface{}{"=", 4, true}})
	if err != nil {
		return err
	} else if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (storage *TTStorage) findActorFollows(id string, follower bool) ([]string, error) {
	idx := "following"
	pos := 1
	if follower {
		idx = "follower"
		pos = 2
	}
	resp, err := storage.db.Select("follow", idx, 0, LIMIT_MSG, tarantool.IterEq, []interface{}{id})
	if err != nil {
		return nil, err
	} else if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	var ids []string
	for _, d := range resp.Data {
		id := d.([]interface{})[pos].(string)
		ids = append(ids, id)
	}
	return ids, nil
}
