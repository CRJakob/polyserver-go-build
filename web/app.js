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

// ---------- CHARTS ----------
let timeLabels = Array(60).fill("");
let histTick = Array(60).fill(0);
let histGoroutine = Array(60).fill(0);
let histMem = Array(60).fill(0);
let histBwUp = Array(60).fill(0);
let histBwDown = Array(60).fill(0);

let chartTick, chartGoroutine, chartMem, chartBw;

function initCharts() {
  const commonOptions = {
    animation: false,
    responsive: true,
    scales: {
      x: { display: false },
      y: { beginAtZero: true }
    },
    elements: {
      point: { radius: 0 }
    }
  };

  chartTick = new Chart(document.getElementById("chart-ticktime"), {
    type: 'line',
    data: {
      labels: timeLabels,
      datasets: [{ label: 'Tick Time (ms)', data: histTick, borderColor: 'rgb(255, 99, 132)', tension: 0.1 }]
    },
    options: commonOptions
  });

  chartGoroutine = new Chart(document.getElementById("chart-goroutines"), {
    type: 'line',
    data: {
      labels: timeLabels,
      datasets: [{ label: 'Goroutines', data: histGoroutine, borderColor: 'rgb(54, 162, 235)', tension: 0.1 }]
    },
    options: commonOptions
  });

  chartMem = new Chart(document.getElementById("chart-memory"), {
    type: 'line',
    data: {
      labels: timeLabels,
      datasets: [{ label: 'Memory (MB)', data: histMem, borderColor: 'rgb(153, 102, 255)', tension: 0.1 }]
    },
    options: commonOptions
  });

  chartBw = new Chart(document.getElementById("chart-bandwidth"), {
    type: 'line',
    data: {
      labels: timeLabels,
      datasets: [
        { label: 'Sent (KB/s)', data: histBwUp, borderColor: 'rgb(75, 192, 192)', tension: 0.1 },
        { label: 'Received (KB/s)', data: histBwDown, borderColor: 'rgb(255, 159, 64)', tension: 0.1 }
      ]
    },
    options: commonOptions
  });
}

let lastBytesSent = 0;
let lastBytesReceived = 0;

async function loadStats() {
  try {
    const r = await fetch("/api/stats");
    const data = await r.json();
    if (data && data.stats) {
      document.getElementById("stats-goroutines").textContent = data.stats.goroutines;
      document.getElementById("stats-ticktime").textContent = (data.stats.tickTime / 1000).toFixed(2) + " ms";
      document.getElementById("stats-memory").textContent = formatBytes(data.stats.memoryAlloc);

      let now = new Date();
      timeLabels.push(`${now.getHours()}:${now.getMinutes()}:${now.getSeconds()}`);
      timeLabels.shift();

      histTick.push(data.stats.tickTime / 1000); histTick.shift();
      histGoroutine.push(data.stats.goroutines); histGoroutine.shift();
      histMem.push(data.stats.memoryAlloc / 1024 / 1024); histMem.shift(); // MB
      
      let upRate = 0;
      let downRate = 0;
      if (lastBytesSent > 0 || lastBytesReceived > 0) {
          upRate = data.stats.bytesSent - lastBytesSent;
          downRate = data.stats.bytesReceived - lastBytesReceived;
      }
      lastBytesSent = data.stats.bytesSent;
      lastBytesReceived = data.stats.bytesReceived;

      histBwUp.push(upRate / 1024); histBwUp.shift(); // KB/s
      histBwDown.push(downRate / 1024); histBwDown.shift(); // KB/s

      // Update multi-bandwidth labels
      const sumArray = (arr, numItems) => arr.slice(-numItems).reduce((a, b) => a + b, 0) * 1024; // Convert KB back to Bytes for formatting
      
      document.getElementById("stats-bw-up-1s").textContent = formatBytes(upRate);
      document.getElementById("stats-bw-up-10s").textContent = formatBytes(sumArray(histBwUp, 10));
      document.getElementById("stats-bw-up-60s").textContent = formatBytes(sumArray(histBwUp, 60));

      document.getElementById("stats-bw-down-1s").textContent = formatBytes(downRate);
      document.getElementById("stats-bw-down-10s").textContent = formatBytes(sumArray(histBwDown, 10));
      document.getElementById("stats-bw-down-60s").textContent = formatBytes(sumArray(histBwDown, 60));

      if (chartTick) {
        chartTick.update();
        chartGoroutine.update();
        chartMem.update();
        chartBw.update();
      }
    }
  } catch (e) {
    document.getElementById("stats-goroutines").textContent = "-";
    document.getElementById("stats-ticktime").textContent = "-";
    document.getElementById("stats-memory").textContent = "-";
    document.getElementById("stats-bw-up-1s").textContent = "-";
    document.getElementById("stats-bw-up-10s").textContent = "-";
    document.getElementById("stats-bw-up-60s").textContent = "-";
    document.getElementById("stats-bw-down-1s").textContent = "-";
    document.getElementById("stats-bw-down-10s").textContent = "-";
    document.getElementById("stats-bw-down-60s").textContent = "-";
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

let currentTracksData = {};
let lastServerMaxPlayers = null;
let lastServerGamemode = null;

async function loadServerData() {
  try {
    const r = await fetch("/api/tracks");
    const data = await r.json();
    let sessionData = JSON.parse(data.session);
    let tracksChanged = JSON.stringify(currentTracksData) !== JSON.stringify(data.tracks);
    currentTracksData = data.tracks;

    inviteBox.textContent = data.invite || "-";
    inviteKeyBox.textContent = data.inviteKey || "-";
    timeoutInBox.textContent = data.timeoutIn || "-";
    const selectDir = document.getElementById("trackDirSelectSession");
    const selectSession = document.getElementById("trackSelectSession");

    if (!sessionData.switchingSession && (tracksChanged || selectDir.children.length == 0)) {
      let oldDir = selectDir.value;
      let oldTrack = selectSession.value;

      selectDir.innerHTML = "";
      for (let folder in data.tracks) {
        const opt = document.createElement("option");
        opt.value = folder;
        opt.textContent = folder;
        selectDir.appendChild(opt);
      }

      if (oldDir && currentTracksData[oldDir]) {
        selectDir.value = oldDir;
      }
      updateTrackDropdown();

      if (oldTrack && selectSession.querySelector(`option[value="${oldTrack}"]`)) {
        selectSession.value = oldTrack;
      }
    }
    
    if (sessionData["maxPlayers"] !== lastServerMaxPlayers) {
      document.getElementById("maxPlayers").value = sessionData["maxPlayers"];
      lastServerMaxPlayers = sessionData["maxPlayers"];
    }

    if (sessionData["gamemode"] !== lastServerGamemode) {
      let index = 0;
      for (let child of document.getElementById("gamemodePicker").children) {
        if (child.children[0]) {
          child.children[0].checked = (index === sessionData["gamemode"]);
        }
        index++;
      }
      lastServerGamemode = sessionData["gamemode"];
    }

    // Auto-Rotate UI Sync
    const autoDir = document.getElementById("autoRotateFolderSelect");
    if (autoDir.children.length == 0 || tracksChanged) {
      let oldAutoDir = autoDir.value;
      autoDir.innerHTML = "";
      for (let folder in data.tracks) {
        const opt = document.createElement("option");
        opt.value = folder;
        opt.textContent = folder;
        autoDir.appendChild(opt);
      }
      if (oldAutoDir && currentTracksData[oldAutoDir]) {
        autoDir.value = oldAutoDir;
      }
    }

    if (data.autorotate) {
        document.getElementById("btnAutoStart").disabled = data.autorotate.enabled;
        document.getElementById("btnAutoStop").disabled = !data.autorotate.enabled;
        document.getElementById("btnAutoSkip").disabled = !data.autorotate.enabled;

        const statStr = document.getElementById("autoRotateStatus");
        if (data.autorotate.enabled) {
            statStr.textContent = `${data.autorotate.state} (Next in ${data.autorotate.timeLeft}s)`;
            statStr.className = "uk-text-success uk-text-bold";
        } else {
            statStr.textContent = "Stopped";
            statStr.className = "uk-text-bold";
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

function updateTrackDropdown() {
  const selectDir = document.getElementById("trackDirSelectSession");
  const selectSession = document.getElementById("trackSelectSession");
  const folder = selectDir.value;
  
  selectSession.innerHTML = "";
  if (currentTracksData[folder]) {
    currentTracksData[folder].forEach((name) => {
      const opt = document.createElement("option");
      opt.value = name;
      opt.textContent = name;
      selectSession.appendChild(opt);
    });
  }
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
      trackDir: document.getElementById("trackDirSelectSession").value,
      track: document.getElementById("trackSelectSession").value,
      maxPlayers: parseInt(document.getElementById("maxPlayers").value),
    }),
  });
  await loadServerData();
}

const delay = ms => new Promise(res => setTimeout(res, ms));

async function quickApplySession() {
  try {
    await fetch("/api/session/end", { method: "POST" });
    await loadServerData();
    console.log("Ended session...");
    
    await delay(1000); // Wait 1s
    
    await sendSession(); // Sets the new payload (this also calls loadServerData)
    console.log("Sent new session data...");
    
    await delay(1000); // Optional slight delay before starting again
    await startSession(); // Triggers propagation to clients
    console.log("Started new session!");
  } catch (err) {
    console.error("Quick Apply Failed:", err);
  }
}

// ---------- AUTO ROTATE ----------

async function startAutoRotate() {
  const folder = document.getElementById("autoRotateFolderSelect").value;
  const interval = parseInt(document.getElementById("autoRotateInterval").value);
  if (!folder || isNaN(interval) || interval < 5) return alert("Invalid interval (minimum 5s) or folder");

  await fetch("/api/autorotate/start", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ folder: folder, interval: interval }),
  });
  await loadServerData();
}

async function stopAutoRotate() {
  await fetch("/api/autorotate/stop", { method: "POST" });
  await loadServerData();
}

async function skipAutoRotate() {
  await fetch("/api/autorotate/skip", { method: "POST" });
  await loadServerData();
}

// ---------- INVITES ----------

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
    document.getElementById("playerCountBadge").textContent = data.players.length;
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
  initCharts();
  loadStats();

  setInterval(updateStatus, 2000);
  setInterval(loadPlayers, 1000);
  setInterval(loadStats, 1000);
  setInterval(loadServerData, 3000);
}

main();
