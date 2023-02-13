#!/usr/bin/env tarantool

box.cfg {listen = 3500}
local function bootstrap()
    print("Start bootstrap")
    local message_id = box.schema.sequence.create('message_id',{start=0,min=0,step=1})
    local messages = box.schema.create_space('messages', {engine = 'vinyl'})
    messages:create_index('primary', {sequence='message_id'})
    messages:format({{name='pk', type='unsigned',is_nullable=false},
                      {name='message', type='string',is_nullable=false},
                      {name='sender', type='string',is_nullable=false}})

    local receivers = box.schema.create_space('receivers', {engine = 'vinyl'})
    receivers:create_index('primary', { unique=true, parts = {
        {field = 1, type = 'unsigned'},
        {field = 2, type = 'string'},
    }})
    receivers:create_index('receivers_idx', { unique=false, parts = {
        {field = 2, type = 'string'},
        {field = 3, type = 'boolean'},
    }})
    receivers:format({
        {name='message_id', type='unsigned',is_nullable=false},
        {name='receiver', type='string', is_nullable=false},
        {name="read",type='boolean', is_nullable=false}
    })
    local liked_id = box.schema.sequence.create('liked_id',{start=0,min=0,step=1})
    local liked = box.schema.create_space('liked', {engine = 'vinyl'})
    liked:format({
        {name='liked_id', type='unsigned',is_nullable=false},
        {name='actor_id', type='string',is_nullable=false},
        {name="object", type='string', is_nullable=false},
        {name="summary", type='string'},
    })
    liked:create_index('primary', {sequence='liked_id'})
    liked:create_index('actors', { unique=false, parts = {
        {field = 2, type = 'string'}
    }})
    liked:create_index('objects', { unique=false, parts = {
        {field = 3, type = 'string'}
    }})

    local follow_id = box.schema.sequence.create('follow_id',{start=0,min=0,step=1})
    local follow = box.schema.create_space('follow', {engine = 'vinyl'})
    follow:format({
        {name='follow_id', type='unsigned',is_nullable=false},
        {name='follower', type='string',is_nullable=false},
        {name="following", type='string', is_nullable=false},
        {name="accepted", type='boolean', is_nullable=false},
    })
    follow:create_index('primary', {sequence='follow_id'})
    follow:create_index('follower', { unique=false, parts = {
        {field = 2, type = 'string'},
        {field = 4, type = 'boolean'},
    }})
    follow:create_index('following', { unique=true, parts = {
        {field = 3, type = 'string'},
        {field = 2, type = 'string'},
    }})
    -- Comment this if you need fine grained access control (without it, guest
    -- will have access to everything)
    -- box.schema.user.grant('guest', 'read,write,execute', 'universe')

    -- Keep things safe by default
    box.schema.user.create('inbox', { password = 'inbox' })
    box.schema.user.grant('inbox', 'replication')
    box.schema.user.grant('inbox', 'read,write,execute', 'space')
    box.schema.user.grant('inbox', 'read,write', 'sequence')
end
box.once('inbox-00', bootstrap)

-- load my_app module and call start() function
-- with some app options controlled by sysadmins
local m = require('inbox').start()
