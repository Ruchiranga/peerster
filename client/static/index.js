hostURL = window.location.href.replace(/\/$/, "");

gossipMessageMap = {};
privateMessageMap = {};
peerNodesList = [];
knownNodesList = [];

totalMessageCount = 0;

function getMessagesFromServer() {
    return $.get(hostURL + "/message", function (data) {
        return data;
    }).fail(function(err) {
        console.log(err);
        clearInterval(timer)
    });
}

function getPeerNodesFromServer() {
    return $.get(hostURL + "/node", function (data) {
        return data;
    }).fail(function(err) {
        console.log(err);
        clearInterval(timer)
    });
}

function getKnownNodesFromServer() {
    return $.get(hostURL + "/origin", function (data) {
        return data;
    }).fail(function(err) {
        console.log(err);
        clearInterval(timer)
    });
}

function getNameFromServer() {
    return $.get(hostURL + "/id", function (data) {
        return data;
    }).fail(function(err) {
        console.log(err);
        clearInterval(timer)
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
            console.log(errorThrown);
            clearInterval(timer)
        }
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
            console.log(errorThrown);
            clearInterval(timer)
        }
    });
}

function sendNodeToSever(address) {
    $.ajax({
        url: hostURL + "/node",
        type: 'post',
        data: address,
        error: function (jqXhr, textStatus, errorThrown) {
            console.log(errorThrown);
            clearInterval(timer)
        }
    });
}

function renderPeerNodes() {
    $.when(getPeerNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, peerNodesList);

        newNodes.forEach(function (node) {
            $("#peers-list").append(`<li class="list-group-item"><b>${node}</b></li>`)
        });

        peerNodesList = nodes;
    })
}

function renderKnownNodes() {
    $.when(getKnownNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, knownNodesList);

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
    $.when(getMessagesFromServer()).then(function(messages) {
        const gossipMap = {};
        const privateMap = {};
        for (const key in messages) {
            if (messages.hasOwnProperty(key)) {
                messages[key].forEach(function (message) {
                    if (message.ID === 0) {
                        privateMap[key] ? privateMap[key].push(message) : privateMap[key] = [message]
                    } else if (message.Text !== '') {
                        gossipMap[key] ? gossipMap[key].push(message) : gossipMap[key] = [message]
                    }
                })
            }
        }

        let newMessages = [];


        for (const key in privateMap) {
            if (privateMap.hasOwnProperty(key)) {
                if (privateMessageMap[key]) {
                    const lengthDiff = privateMap[key].length - privateMessageMap[key].length;
                    const news = privateMap[key].slice(Math.max(privateMap[key].length - lengthDiff, 0));
                    newMessages.push(...news)
                } else {
                    newMessages.push(...privateMap[key])
                }
            }
        }
        for (const key in gossipMap) {
            if (gossipMap.hasOwnProperty(key)) {
                if (gossipMessageMap[key]) {
                    const lengthDiff = gossipMap[key].length - gossipMessageMap[key].length;
                    const news = gossipMap[key].slice(Math.max(gossipMap[key].length - lengthDiff, 0));
                    newMessages.push(...news)
                } else {
                    newMessages.push(...gossipMap[key])
                }
            }
        }

        newMessages.forEach(function(rumor) {
            if (rumor.Text !== '') {
                const privateBadge = '<span class="badge badge-secondary">PRIVATE</span>';
                const seqNumber = `(Seq ${rumor.ID})`;
                $("#chat-msgs-list").append(
                    `<div><strong>${rumor.Origin} ${rumor.ID === 0 ? privateBadge : seqNumber}:</strong> ${rumor.Text}</div>`);
            }
        });

        gossipMessageMap = gossipMap;
        privateMessageMap = privateMap;
    });
}

function renderName() {
    $.when(getNameFromServer()).then(function (name) {
        $("#id-placeholder").html(`<b>${name}</b>`);
    });
}

function onClickGossipMessageSend () {
    const textArea = $("#send-message-txtarea");
    const text = textArea.val();
    sendGossipMessageToServer(text);
    textArea.val('');
}

function onClickPrivateMessageSend () {
    const dest = $(".active")[0].text;
    const textArea = $("#send-pm-txtarea");
    const text = textArea.val();
    sendPrivateMessageToServer(dest, text);
    $("#pm-modal").modal("hide");
    textArea.val('');
}

function onClickNodeSend () {
    const textBox = $("#send-node-txt");
    const address = textBox.val();
    sendNodeToSever(address);
    textBox.val('');
}

$("#send-message-txtarea").keypress(function (e) {
    if(e.which === 13) {
        onClickGossipMessageSend();
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

timer = setInterval(function(){
    renderMessages();
    renderPeerNodes();
    renderKnownNodes();
}, 1000);