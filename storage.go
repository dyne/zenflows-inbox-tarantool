package main

import (
	"fmt"
	"encoding/json"
	//"time"
	"github.com/tarantool/go-tarantool"
	//"log"
)

type TTStorage struct {
	db  *tarantool.Connection
}

type ReadAll struct {
	Id      int                    `json:"id"`
	Sender  string                 `json:"sender"`
	Content map[string]interface{} `json:"content"`
	Read    bool                   `json:"read"`
}

const MAX_RETRY int = 10

func (storage *TTStorage) init(connectionStr string) error {
	var err error
	storage.db, err = tarantool.Connect("127.0.0.1:3500", tarantool.Opts{
		User: "inbox",
		Pass: "inbox",
	})
	if err != nil {
		return err
	}

	return nil
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
	resp, err := storage.db.Select("receivers", "receivers_idx", 0, 4096, tarantool.IterEq, []interface{}{who})
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
			Id: int(dataRead[0].(uint64)),
			Sender: dataRead[2].(string),
			Read: read,
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
	resp, err := storage.db.Update("receivers", "primary", []interface{}{uint64(message_id), who}, []interface{}{[]interface{}{"=", 2, read}})
	if err != nil {
		return err
	}
	return nil
}

func (storage *TTStorage) countUnread(who string) (int, error) {
	return 0, nil
}
