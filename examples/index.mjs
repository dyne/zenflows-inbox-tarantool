#!/usr/bin/env node
import sign from "./sign_graphql.mjs"
import { zencode_exec } from 'zenroom';
import axios from 'axios';

const PIPPO_EDDSA = "EtJtSqAG9mVHfKrKduS6aeyAE6okGXrfMW8fEQ6eqenh"
const PIPPO_ID = "062TE0H7591KJCVT3DDEMDBF0R"
const PLUTO_EDDSA = "2n4TEhoQ8ZwedJoUuJNbxv5W1cr5wHFYPcQmkk1EWj4t"
const PLUTO_ID = "062TE0YPJD392CS1DPV9XWMDXC"
const PAPERINO_EDDSA = "H7sbugVBZbmRX6M75WpzCi5vVVtaxvfLhovDijRAnZj"
const PAPERINO_ID = "062TE18QJSQJ1PY6G1M7783148"

const url="http://localhost:5000"
//const url="https://gateway0.interfacer.dyne.org/inbox"

const signRequest = async (json, key) => {
	const data = `{"gql": "${Buffer.from(json, 'utf8').toString('base64')}"}`
    const keys = `{"keyring": {"eddsa": "${key}"}}`
	const {result} = await zencode_exec(sign(), {data, keys});
	return {
		'zenflows-sign': JSON.parse(result).eddsa_signature
	}
}

const sendMessage = async (message) => {
    const request = {
        sender: PIPPO_ID,
        receivers: [PAPERINO_ID,PLUTO_ID],
        content: {
            message: message,
            subject: "Subject",
            data:    "timestamp"
        }
    }
    const requestJSON = JSON.stringify(request)
    const requestHeaders =  await signRequest(requestJSON, PIPPO_EDDSA);
    const config = {
        headers: requestHeaders
    };

    const result = await axios.post(`${url}/send`, request, config);
    return result
}

const assertPostMany = async() => {
    for(let i=0; i<10; i++) {
        const res = await sendMessage(`Ciao a tutti ${i}`)
    console.log(res)

        console.assert(res.data.success)
        console.assert(res.data.count == 2)
    }
}

const readMessages = async(email, key) => {
    const request = {
        request_id: 42,
        receiver: email,
        //only_unread: true,
    }
    const requestJSON = JSON.stringify(request)
    const requestHeaders =  await signRequest(requestJSON, key);
    const config = {
        headers: requestHeaders
    };

    const result = await axios.post(`${url}/read`, request, config);
    return result
}

const assertReadMany = async(email, key) => {
    const res = await readMessages(email, key)
    console.assert(res.data.success)
    res.data.messages.forEach((v, i) => {
        console.log(v)
        console.assert(v.content.message.startsWith("Ciao a tutti"))
        console.assert(v.content.subject == "Subject")
        console.assert(!v.read)
    })

}

const setMessage = async(message_id, receiver, read, key) => {
    const request = {
        message_id,
        receiver,
        read
    }
    const requestJSON = JSON.stringify(request)
    const requestHeaders =  await signRequest(requestJSON, key);
    const config = {
        headers: requestHeaders
    };

    const result = await axios.post(`${url}/set-read`, request, config);
    return result
}

const countMessages = async(receiver, key) => {
    const request = {
        receiver,
    }
    const requestJSON = JSON.stringify(request)
    const requestHeaders =  await signRequest(requestJSON, key);
    const config = {
        headers: requestHeaders
    };

    const result = await axios.post(`${url}/count-unread`, request, config);
    return result
}

const deleteMessage = async(receiver, messageId, key) => {
    const request = {
        receiver,
        message_id: messageId,
    }
    const requestJSON = JSON.stringify(request)
    const requestHeaders =  await signRequest(requestJSON, key);
    const config = {
        headers: requestHeaders
    };

    const result = await axios.post(`${url}/delete`, request, config);
    return result
}
await assertPostMany()
assertReadMany(PLUTO_ID, PLUTO_EDDSA)
//assertReadMany(PAPERINO_ID, PAPERINO_EDDSA)
//setMessage(10, PLUTO_ID, true, PLUTO_EDDSA)
//console.log(await countMessages(PLUTO_ID, PLUTO_EDDSA))
//deleteMessage(PLUTO_ID, 18, PLUTO_EDDSA)
