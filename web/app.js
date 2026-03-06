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

function formatBytes(bytes, decimals = 2) {
  if (!+bytes) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

async function loadStats() {
  try {
    const r = await fetch("/api/stats");
    const data = await r.json();
    if (data && data.stats) {
      document.getElementById("stats-goroutines").textContent = data.stats.goroutines;
      document.getElementById("stats-memory").textContent = formatBytes(data.stats.memoryAlloc);
      document.getElementById("stats-bw-up").textContent = formatBytes(data.stats.bytesSent);
      document.getElementById("stats-bw-down").textContent = formatBytes(data.stats.bytesReceived);
    }
  } catch (e) {
    document.getElementById("stats-goroutines").textContent = "-";
    document.getElementById("stats-memory").textContent = "-";
    document.getElementById("stats-bw-up").textContent = "-";
    document.getElementById("stats-bw-down").textContent = "-";
  }
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
    const selectSession = document.getElementById("trackSelectSession");

    if (!sessionData.switchingSession || selectSession.children.length == 0) {
      selectSession.innerHTML = "";
      for (let key in data.tracks) {
        data.tracks[key].forEach((name) => {
          const opt2 = document.createElement("option");
          opt2.value = `${key}/${name}`;
          opt2.textContent = `${key}/${name}`;;

          selectSession.appendChild(opt2);
        });
      }
    }
    let sessionInfoDiv = document.getElementById("sessionInfo")
    sessionInfoDiv.innerHTML = `
      <p>Session ID: <strong>${sessionData["sessionId"]}</strong></p>
      <p>Session Gamemode: <strong>${sessionData["gamemode"] == 1 ? "Competitive" : "Casual"}</strong></p>
      <p>Max players: <strong>${sessionData["maxPlayers"]}</strong></p>
      <p>Current Map: <b>${data.currentDir}/${data.current}</b></p>
      `;
    document.getElementById("startSessionBtn").disabled = !sessionData["switchingSession"]
    document.getElementById("sendSessionBtn").disabled = !sessionData["switchingSession"] || sessionData["propagated"]
    document.getElementById("endSessionBtn").disabled = sessionData["switchingSession"]
  } catch (e) {
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
  for (let child of document.getElementById("gamemodePicker").children) {
    if (child.children[0].checked) break;
    index++;
  }
  await fetch("/api/session/set", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      gamemode: index,
      trackDir: document.getElementById("trackSelectSession").value.split("/")[0],
      track: document.getElementById("trackSelectSession").value.split("/")[1],
      maxPlayers: parseInt(document.getElementById("maxPlayers").value),
    }),
  });
  await loadServerData()
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

async function reloadTracks() {
  await fetch("/api/reloadTracks", {
    method: "POST"
  });
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
  loadStats();

  setInterval(updateStatus, 2000);
  setInterval(loadPlayers, 1000);
  setInterval(loadStats, 1000);
  setInterval(loadServerData, 3000);
}

main();
