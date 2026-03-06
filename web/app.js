const inviteBox = document.getElementById("invite");
const inviteKeyBox = document.getElementById("inviteKey");
const timeoutInBox = document.getElementById("timeoutIn");

async function updateStatus() {
  const r = await fetch("/api/server/status");
  const data = await r.json();

  document.getElementById("status").textContent = data.running
    ? "Running"
    : "Stopped";

  document.getElementById("pid").textContent = data.running ? data.pid : "-";
}

async function startServer() {
  await fetch("/api/server/start", { method: "POST" });

  setTimeout(() => {
    updateStatus();
    loadServerData();
  }, 800);
}

async function stopServer() {
  await fetch("/api/server/stop", { method: "POST" });
  setTimeout(updateStatus, 500);
}

// ---------- INVITE + TRACKS ----------

async function loadServerData() {
  try {
    const r = await fetch("/api/tracks");
    const data = await r.json();
    let sessionData = JSON.parse(data.session);

    inviteBox.textContent = data.invite || "-";
    inviteKeyBox.textContent = data.inviteKey || "-";
    timeoutInBox.textContent = data.timeoutIn || "-";

    const select = document.getElementById("trackSelect");
    const selectSession = document.getElementById("trackSelectSession");

    if(!sessionData.switchingSession || selectSession.children.length == 0) {
      select.innerHTML = "";
      selectSession.innerHTML = "";
      data.tracks.forEach((name) => {
        const opt = document.createElement("option");
        opt.value = name;
        opt.textContent = name;

        if (name === data.current) opt.selected = true;

        const opt2 = document.createElement("option");
        opt2.value = name;
        opt2.textContent = name;

        select.appendChild(opt);
        selectSession.appendChild(opt2);
      });
    }
    let sessionInfoDiv = document.getElementById("sessionInfo")
    sessionInfoDiv.innerHTML = `
      <p>Session ID: <strong>${sessionData["sessionId"]}</strong></p>
      <p>Session Gamemode: <strong>${sessionData["gamemode"] == 1 ? "Competitive" : "Casual"}</strong></p>
      <p>Max players: <strong>${sessionData["maxPlayers"]}</strong></p>
      <p>Switching sessions? <strong>${sessionData["switchingSession"] ? "Yes" : "No"}</strong></p>
      `;
      document.getElementById("startSessionBtn").disabled = !sessionData["switchingSession"]
      document.getElementById("sendSessionBtn").disabled = !sessionData["switchingSession"]
      document.getElementById("endSessionBtn").disabled = sessionData["switchingSession"]

  } catch(e) {
    console.log("Error " + e)
    inviteBox.textContent = "(server not running)";
  }
}

async function endSession() {
  const r = await fetch("/api/session/end", { method: "POST" });
  await loadServerData()
}
async function startSession() {
  const r = await fetch("/api/session/start", { method: "POST" });
  await loadServerData()
}

async function sendSession() {
  let index = 0;
  for(let child of document.getElementById("gamemodePicker").children) {
    if(child.children[0].checked) break;
    index++;
  }
  await fetch("/api/session/set", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ 
      gamemode: index, 
      track: document.getElementById("trackSelectSession").value,
      maxPlayers: parseInt(document.getElementById("maxPlayers").value),
    }),
  });
}

async function createInvite(regenerate) {
  const r = await fetch("/api/invite", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      regenerate: regenerate ? true : false,
      key: "nu"
    }),
  });
  const data = await r.json();

  inviteBox.textContent = data.invite;
  inviteKeyBox.textContent = data.inviteKey;
  timeoutInBox.textContent = data.timeoutIn;
  await loadServerData();
}

// ---------- PLAYERS ----------

async function loadPlayers() {
  try {
    const r = await fetch("/api/players");
    const data = await r.json();

    const tbody = document.querySelector("#players tbody");
    tbody.innerHTML = "";
    data.players.forEach((p) => {
      const tr = document.createElement("tr");

      tr.innerHTML = `
        <td>${p.name}</td>
        <td>${p.time}</td>
        <td>${p.ping} ms</td>
        <td><button class="uk-button uk-button-danger" type="button" onclick="kickPlayer(${p.id})">Kick</button></td>
      `;

      tbody.appendChild(tr);
    });
  } catch {
    // server not running
  }
}

async function kickPlayer(id) {
  await fetch("/api/kick", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
  });
}

// ---------- INIT ----------

function main() {
  updateStatus();
  loadServerData();
  loadPlayers();

  setInterval(updateStatus, 2000);
  setInterval(loadPlayers, 1000);
  setInterval(loadServerData, 3000);
}

main();
