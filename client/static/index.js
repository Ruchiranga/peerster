hostURL = "http://localhost:8080";

messagesMap = {};
nodesList = [];

function getMessagesFromServer() {
    return $.get(hostURL + "/message", function (data) {
        return data;
    });
}

function getNodesFromServer() {
    return $.get(hostURL + "/node", function (data) {
        return data;
    });
}

function getNameFromServer() {
    return $.get(hostURL + "/id", function (data) {
        return data;
    });
}

function sendMessageToServer(text) {
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
    $.when(getNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, nodesList);
        console.log(`New nodes: ${JSON.stringify(newNodes)}`);

        newNodes.forEach(function (node) {
            $("#nodes-list").append(`<li class="list-group-item"><b>${node}</b></li>`)
        });

        nodesList = nodes;
    })

}

function renderMessages() {
    $.when(getMessagesFromServer()).then(function(messages) {
        console.log(`All messages ${JSON.stringify(messages)}`);
        const filteredMessages = _.omitBy(messages, function(item) {
            if (JSON.stringify(messagesMap[item[0].Origin]) === JSON.stringify(item)) return true;
        });

        let newMessages = [];
        for (const key in filteredMessages) {
            if (filteredMessages.hasOwnProperty(key)) {
                const diff = filteredMessages[key].length - ((messagesMap[key] && messagesMap[key].length) || 0);
                const news = filteredMessages[key].slice(Math.max(filteredMessages[key].length - diff, 0));
                newMessages.push(...news)
            }
        }
        console.log(`New messages: ${JSON.stringify(newMessages)}`);

        newMessages.forEach(function(rumor) {
            $("#chat-msgs-list").append(`<div><strong>${rumor.Origin} (Seq ${rumor.ID}) :</strong> ${rumor.Text}</div>`);
        });

        messagesMap = messages;
    });
};

function renderName() {
    $.when(getNameFromServer()).then(function (name) {
        $("#id-placeholder").html(`<b>${name}</b>`);
    });
};

function onClickMessageSend () {
    const text = $("#send-message-txtarea").val();
    sendMessageToServer(text);
    $("#send-message-txtarea").val('');
}

function onClickNodeSend () {
    const address = $("#send-node-txt").val();
    sendNodeToSever(address);
    $("#send-node-txt").val('');
}

$("#send-message-txtarea").keypress(function (e) {
    if(e.which == 13) {
        onClickMessageSend();
        e.preventDefault();
    }
});

renderName();

window.setInterval(function(){
    renderMessages();
    renderNodes();
}, 1000);