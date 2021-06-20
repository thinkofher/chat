const chatURL = "wss://" + window.location.host + "/chat";

const chatWindow = document.getElementById("chat-window");
const inputForm = document.getElementById("input-form");

async function appendMessage({ nick, message }) {
  chatWindow.innerHTML += `<p><b>${nick}</b>: ${message}</p>`;
  chatWindow.scrollTo(0, chatWindow.scrollHeight);
}

const socket = new WebSocket(chatURL);

inputForm.addEventListener("submit", (e) => {
  e.preventDefault();

  const data = {
    nick: e.target.nick.value,
    message: e.target.msg.value,
  };
  e.target.msg.value = "";

  if (data.message != "") {
    socket.send(JSON.stringify(data));
  }
});

socket.addEventListener("message", (e) => {
  appendMessage(JSON.parse(e.data));
});

socket.addEventListener("error", (e) => {
  console.log("web socket error");
});
