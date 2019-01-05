hostURL = window.location.href.replace(/\/$/, "");

gossipMessageMap = {};
privateMessageMap = {};
peerNodesList = [];
knownNodesList = [];

totalMessageCount = 0;

gossiperName = "";

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

function getSearchResultsFromServer(keywords, cb) {
    const xhr = new XMLHttpRequest();
    xhr.open("GET", `/search?keywords=${keywords}`, true);
    xhr.onprogress = function () {
        cb(xhr.responseText)
    };
    xhr.onload = function () {
        console.log(`Connection for keywords ${keywords} closed`)
    };
    xhr.send();
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
        }
    });
}

function sendSecurePrivateMessageToServer(dest, text) {
    const message = {"encprivate": {"temp": text, "Destination": dest}};
    $.ajax({
        url: hostURL + "/message",
        type: 'post',
        contentType: 'application/json',
        data: JSON.stringify(message),
        error: function (jqXhr, textStatus, errorThrown) {
            console.log(errorThrown);
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
        }
    });
}

function sendIndexFileNameToServer(fileName) {
    return $.ajax({
        url: hostURL + "/file",
        type: 'post',
        data: fileName,
        error: function (jqXhr, textStatus, errorThrown) {
            console.log(errorThrown);
        }
    });
}

function sendFileDownloadRequestToServer(fileName, destination, hash) {
    let pathURL = `/file?fileName=${fileName}&metaHash=${hash}`;
    if (destination) {
        pathURL = `${pathURL}&destination=${destination}`
    }
    return $.get(hostURL + pathURL, function (data) {
        return data;
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
            $('#secure-private-msg-btn').removeAttr('disabled');
            $('#file-download-btn').removeAttr('disabled');
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
    return $.when(getNameFromServer()).then(function (name) {
        $("#id-placeholder").html(`<b>${name}</b>`);

        // For testing purposes
        gossiperName = name;
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

function onClickSecureMessageSend () {
    const dest = $(".active")[0].text;
    const textArea = $("#send-secure-pm-txtarea");
    const text = textArea.val();
    sendSecurePrivateMessageToServer(dest, text);
    $("#secure-pm-modal").modal("hide");
    textArea.val('');
}

function onClickNodeSend () {
    const textBox = $("#send-node-txt");
    const address = textBox.val();
    sendNodeToSever(address);
    textBox.val('');
}

function onClickIndexFile () {
    const fileName = $('#file-input')[0].files[0].name;
    $.when(sendIndexFileNameToServer(fileName)).then(
        function(success) {
            if (success === 'true') {
                $('#file-index-status').html(`<div class="alert alert-success">File Indexed successfully</div>`);
            } else {
                $('#file-index-status').html(`<div class="alert alert-danger">File Indexing failed. Please select a file inside _SharedFiles directory</div>`);
            }
        }
    );
}

// Only used for testing purposes
function clickIndexFileManual (name) {
    $.when(sendIndexFileNameToServer(name)).then(
        function(success) {
            if (success === 'true') {
                $('#file-index-status').html(`<div class="alert alert-success">File Indexed successfully</div>`);
            } else {
                $('#file-index-status').html(`<div class="alert alert-danger">File Indexing failed. Please select a file inside _SharedFiles directory</div>`);
            }
        }
    );
}

function escapeRegExp(text) {
    return text.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, '\\$&');
}

function onClickSearchFile () {
    $('#search-results-list').html('');
    const textBox = $("#file-search-input");
    const keywords = textBox.val();

    const cb = function(fileNames) {
        $('#search-results-list').html('');
        const lines = fileNames.split('\n');
        console.log(lines);
        lines.forEach(function (line) {
            if (line !== "") {
                const splits = line.split(',');
                const fileName = splits[0];
                const hash = splits[1];
                const origin = splits[2];
                keywords.split(',').forEach(function (keyword) {
                    const regex = new RegExp('^.*' + escapeRegExp(keyword) + '.*$', 'g');
                    if (fileName.match(regex)) {
                        $('#search-results-list')
                            .append(`<button type="button" class="list-group-item" data-metahash="${hash}" ondblclick="onClickSearchDownload(this)">${fileName}${origin ? `<span class="play-button" class="list-group-item" data-metahash="${hash}" data-filename="${fileName}" data-origin="${origin}" ondblclick="event.stopPropagation()" onclick="onClickStream(this)">▶</span>` : ''}</button>`);
                    } else {
                        console.log(`Received result ${fileName} doesn't match the keywords`)
                    }
                })
            }
        });
    };
    getSearchResultsFromServer(keywords, cb)
}

function onClickTriggerDownload () {
    const dest = $(".active")[0].text;
    const fileNameTxt =  $("#file-name-txt");
    const metaHashTxt = $("#file-hash-txt");
    const fileName = fileNameTxt.val();
    const methash = metaHashTxt.val();
    $('#file-download-status').html('<div id="download-spinner"></div>');
    $.when(sendFileDownloadRequestToServer(fileName, dest, methash)).then(function (status) {
        if (status) {
            $('#file-download-status').html(`<div id="download-status-alert" class="alert alert-success fade in">File download completed</div>`);
            setTimeout(function () {
                $('#download-status-alert').alert('close')
            }, 3000)
        }
    }).fail(function(err) {
        console.log(err);
        $('#file-download-status').html(`<div id="download-status-alert" class="alert alert-danger fade in">File download failed</div>`);
        setTimeout(function () {
            $('#download-status-alert').alert('close')
        }, 3000)
    });
    $("#file-modal").modal("hide");
    fileNameTxt.val('');
    metaHashTxt.val('');
}

function onClickSearchDownload (element) {
    const fileName = element.innerHTML;
    const methash = element.dataset.metahash;
    $('#file-download-status').html('<div id="download-spinner"></div>');
    $.when(sendFileDownloadRequestToServer(fileName, undefined, methash)).then(function (status) {
        if (status) {
            $('#file-download-status').html(`<div id="download-status-alert" class="alert alert-success fade in">File download completed</div>`);
            setTimeout(function () {
                $('#download-status-alert').alert('close')
            }, 3000)
        }
    }).fail(function(err) {
        console.log(err);
        $('#file-download-status').html(`<div id="download-status-alert" class="alert alert-danger fade in">File download failed</div>`);
        setTimeout(function () {
            $('#download-status-alert').alert('close')
        }, 3000)
    });
}

function onClickStream(element, b, c, d) {
    const fileName = element.dataset.filename,
        mimeType = 'audio/mpeg',
        assetURL = element.dataset.origin,
        segmentSize = 1024 * 1024, // 1 MB
        nameWithoutExtension = fileName.replace(/\.[^/.]+$/, "");
    $('#player-container').show();
    $('#player-name').html(nameWithoutExtension);
    let player = document.getElementById("audio-player"),
        lastFetched = 0,
        fileSize = 0,
        segmentsList = [],
        sourceBuffer = null,
        mediaSource = null;

    if ('MediaSource' in window && MediaSource.isTypeSupported(mimeType)) {
        mediaSource = new MediaSource;
        player.src = URL.createObjectURL(mediaSource);
        mediaSource.addEventListener('sourceopen', onSourceOpen);
    } else {
        console.error('Unsupported MIME type or codec: ', mimeType);
    }
    // player.src = origin;
    // player.play();

    function onSourceOpen() {
        sourceBuffer = mediaSource.addSourceBuffer(mimeType);
        getFileLength(assetURL, function (fileLength) {
            fileSize = fileLength;
            console.log("size: ", fileLength);
            console.log((fileSize / 1024 / 1024).toFixed(2), 'MB');
            let auxByte = 0,
                numberOfSegments = Math.ceil(fileSize / segmentSize);
            for (let i = 0; i < numberOfSegments; i++) {
                segmentsList[i] = {
                    start: auxByte,
                    end: i === numberOfSegments - 1 ? auxByte + fileSize % segmentSize - 1 : auxByte + segmentSize - 1,
                };
                auxByte += segmentSize;
            }
            console.log(segmentsList);
            fetchRange(assetURL, segmentsList[lastFetched].start, segmentsList[lastFetched].end);
            player.addEventListener('canplay', function () {
                player.play();
            });
            player.onloadedmetadata = function () {
                console.log('onlodademetadata:', player.duration);
            };
        });
    }


     function getFileLength(url, cb) {
        var xhr = new XMLHttpRequest;
        xhr.open('head', url);
        xhr.onload = function () {
            cb(xhr.getResponseHeader('Content-Length'));
        };
        xhr.send();
    }

    function fetchRange(url, start, end) {
        let xhr = new XMLHttpRequest;
        xhr.open('get', url);
        xhr.responseType = 'arraybuffer';
        xhr.setRequestHeader('Range', 'bytes=' + start + '-' + end);
        xhr.setRequestHeader('Accept', '*/*');
        xhr.onload = function () {
            console.log('Fetched range: ', start, end);
            try {
                sourceBuffer.appendBuffer(xhr.response);
            }
            catch(err) {
                console.log("Stream no longer valid");
                return;
            }
            lastFetched++;
            if (lastFetched < segmentsList.length) {
                const sleep = (milliseconds) => {
                    return new Promise(resolve => setTimeout(resolve, milliseconds))
                };
                sleep(1000).then(() => {
                    if (segmentsList[lastFetched]) {
                        fetchRange(url, segmentsList[lastFetched].start, segmentsList[lastFetched].end);
                    }
                });
            }
        };
        xhr.send();
    }
}

$(function() {
    $("form").submit(function(e) {
        e.preventDefault();
    });

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

    $("#file-search-input").keyup(function () {
        if ($(this).val()) {
            $('#search-file-btn').removeAttr('disabled');
        } else {
            $('#search-file-btn').prop('disabled', true);
        }
    });

    $("#file-input").change(function(){
        if ($(this).val()) {
            $('#index-file-btn').removeAttr('disabled');
        } else {
            $('#index-file-btn').prop('disabled', true);
        }
        $('#file-index-status').html('')
    });

    renderName().then(function () {
        // Testing code

        // if (gossiperName === 'two' || gossiperName === 'eight' || gossiperName === 'five') {
        //     clickIndexFileManual('test_ring_3_1.txt')
        // }
        // if (gossiperName === 'three') {
        //     clickIndexFileManual('test_ring_3_2.txt')
        // }
        // if (gossiperName === 'four') {
        //     clickIndexFileManual('test.txt')
        // }
        //
        //
        // const data = {
        //     one: 1,
        //     two: 2,
        //     three:3,
        //     four:4,
        //     five:5,
        //     six:6,
        //     seven:7,
        //     eight:8,
        //     nine:9
        // };
        // let ptr = 0
        // window.setInterval(function(){
        //     const keys = Object.keys(data)
        //     const key = keys[ptr]
        //     if (gossiperName === key) {
        //         console.log(`Indexing ${data[key]}.txt of ${key}`)
        //         clickIndexFileManual(`${data[key]}.txt`)
        //     }
        //     ptr++;
        // }, 1000);
    });

    timer = setInterval(function(){
        renderMessages();
        renderPeerNodes();
        renderKnownNodes();
    }, 1000);
});
