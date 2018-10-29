hostURL = window.location.href.replace(/\/$/, "");

messagesMap = {};
nodesList = [];

totalMessageCount = 0;

function getMessagesFromServer() {
    return $.get(hostURL + "/message", function (data) {
        return data;
    }).fail(function(err) {
        console.log(err);
        clearInterval(timer)
    });
}

function getNodesFromServer() {
    return $.get(hostURL + "/node", function (data) {
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

function sendMessageToServer(text) {
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

function renderNodes() {
    $.when(getNodesFromServer()).then(function(nodes) {
        const newNodes = _.difference(nodes, nodesList);
        // console.log(`New nodes: ${JSON.stringify(newNodes)}`);

        newNodes.forEach(function (node) {
            $("#nodes-list").append(`<li class="list-group-item"><b>${node}</b></li>`)
        });

        nodesList = nodes;
    })

}

function renderMessages() {
    $.when(getMessagesFromServer()).then(function(messages) {
        // console.log(`All messages ${JSON.stringify(messages)}`);
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
        // console.log(`New messages: ${JSON.stringify(newMessages)}`);
        totalMessageCount += newMessages.length;
        console.log(`${totalMessageCount} ${count === threshold ? 'im done' : ''}`);
        newMessages.forEach(function(rumor) {
            $("#chat-msgs-list").append(`<div><strong>${rumor.Origin} (Seq ${rumor.ID}) :</strong> ${rumor.Text}</div>`);
        });

        messagesMap = messages;
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
    sendMessageToServer(text);
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

threshold = 30;
count = 0;
timer = setInterval(function(){
    renderMessages();
    renderNodes();

    if (count < threshold) {
        sendMessageToServer(`${Math.random() * 100}`)
        count += 1;
    }

}, 1000);