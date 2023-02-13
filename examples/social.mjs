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

const useLikeObject = async () => {
  const request = {
    "@context": "https://www.w3.org/ns/activitystreams",
    "type": "Like",
    "actor": "https://gatewat0.interfacer.dyne.org/inbox/social/062TE0H7591KJCVT3DDEMDBF0R",
    "object": "https://gateway0.interfacer.dyne.org/inbox/economicresource/062SE9RG34DDHTEHHRWY8VKCJW",
    "published": "2014-09-30T12:34:56Z"
  }
  const requestJSON = JSON.stringify(request)
  const requestHeaders = await signRequest(requestJSON, PIPPO_EDDSA);
  const config = {
    headers: requestHeaders
  };

  const result = await axios.post(`${url}/social/062TE0H7591KJCVT3DDEMDBF0R/outbox`, request, config);
  return result
}

console.log(await useLikeObject())
