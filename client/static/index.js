hostURL = window.location.href.replace(/\/$/, "");

gossipMessagesMap = {};
privateMessagesMap = {};
peerNodesList = [];
knownNodesList = [];

DEBUG = false;

function getGossipMessagesFromServer() {
    return $.get(hostURL + "/message", function (data) {
        return data;
    });
}

function getPrivateMessagesFromServer() {
    return $.get(hostURL + "/private", function (data) {
        return data;
    });
}

function getPeerNodesFromServer() {
    return $.get(hostURL + "/node", function (data) {
        return data;
    });
}

function getKnownNodesFromServer() {
    return $.get(hostURL + "/origins", function (data) {
        return data;
    });
}

function getNameFromServer() {
    return $.get(hostURL + "/id", function (data) {
        return data;
    });
}

function sendGossipMessageToServer(text) {
    const rumour = {"Rumor": {"Text": text}};

    $.ajax({
        url: hostURL + "/message",
        type: 'post',
        contentType: 'application/json',
        data: JSON.stringify(rumour),
        error: function (jqXhr, textStatus, errorThrown) {
            alert(errorThrown);
        }
    });
}

function sendPrivateMessageToServer(dest, text) {
    const message = {"Private": {"Text": text, "Destination": dest}};

    $.ajax({
        url: hostURL + "/message",
        type: 'post',
        contentType: 'application/json',
        data: JSON.stringify(message),
        error: function (jqXhr, textStatus, errorThrown) {
            alert(errorThrown);
        }
    });
}

function sendNodeToSever(address) {
    $.ajax({
        url: hostURL + "/node",
        type: 'post',
        data: address,
        error: function (jqXhr, textStatus, errorThrown) {
            alert(errorThrown);
        }
    });
}

function renderNodes() {
    $.when(getPeerNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, peerNodesList);
        if (DEBUG) console.log(`New peer nodes: ${JSON.stringify(newNodes)}`);

        newNodes.forEach(function (node) {
            $("#peers-list").append(`<li class="list-group-item"><b>${node}</b></li>`)
        });

        peerNodesList = nodes;
    })
}

function renderOrigins() {
    $.when(getKnownNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, knownNodesList);
        if (DEBUG) console.log(`New known nodes: ${JSON.stringify(newNodes)}`);

        newNodes.forEach(function (node) {
            $("#known-nodes-list").append(`<a class="list-group-item origin-item"><b>${node}</b></a>`)
        });


        $(".origin-item").on('click', function() {
            $('.active').removeClass('active');
            $(this).toggleClass('active');
            $('#private-msg-btn').removeAttr('disabled');
        });

        knownNodesList = nodes;
    })
}

function renderMessages() {
    const render = function(nextMessages, previousMessages) {
        if (DEBUG) console.log(`All messages ${JSON.stringify(nextMessages)}`);
        const filteredMessages = _.omitBy(nextMessages, function(item) {
            if (JSON.stringify(previousMessages[item[0].Origin]) === JSON.stringify(item)) return true;
        });

        let newMessages = [];
        for (const key in filteredMessages) {
            if (filteredMessages.hasOwnProperty(key)) {
                const diff = filteredMessages[key].length - ((previousMessages[key] && previousMessages[key].length) || 0);
                const news = filteredMessages[key].slice(Math.max(filteredMessages[key].length - diff, 0));
                newMessages.push(...news)
            }
        }
        if (DEBUG) console.log(`New messages: ${JSON.stringify(newMessages)}`);

        newMessages.forEach(function(rumor) {
            if (rumor.Text !== '') {
                $("#chat-msgs-list").append(`<div><strong>${rumor.Origin} (Seq ${rumor.ID}) :</strong> ${rumor.Text}</div>`);
            }
        });
    };

    $.when(getGossipMessagesFromServer()).then(function (messages) {
        render(messages, gossipMessagesMap);
        gossipMessagesMap = messages;
    });
    $.when(getPrivateMessagesFromServer()).then(function (messages) {
        render(messages, privateMessagesMap);
        privateMessagesMap = messages;
    });
}

function renderName() {
    $.when(getNameFromServer()).then(function (name) {
        $("#id-placeholder").html(`<b>${name}</b>`);
    });
}

function onClickMessageSend () {
    const textArea = $("#send-message-txtarea");
    const text = textArea.val();
    sendGossipMessageToServer(text);
    textArea.val('');
}

function onClickNodeSend () {
    const textBox = $("#send-node-txt");
    const address = textBox.val();
    sendNodeToSever(address);
    textBox.val('');
}

function onClickPMSend () {
    const dest = $(".active")[0].text;
    const textArea = $("#send-pm-txtarea");
    const text = textArea.val();
    sendPrivateMessageToServer(dest, text);
    $("#pm-modal").modal("hide");
    textArea.val('');
}

$("#send-message-txtarea").keypress(function (e) {
    if(e.which === 13) {
        onClickMessageSend();
        e.preventDefault();
    }
});

$("#send-node-txt").keypress(function (e) {
    if(e.which === 13) {
        onClickNodeSend();
        e.preventDefault();
    }
});

renderName();

window.setInterval(function(){
    renderMessages();
    renderNodes();
    renderOrigins();
}, 1000);