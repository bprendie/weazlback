.pragma library

function emptyStatus() {
  return {schema_version: 2, state: "not-configured", vault_state: "locked", percent: 0,
    last_healthy: "", destination: "", profiles: [], travel_until: "", updated_at: ""}
}

function parse(text) {
  try {
    var value = JSON.parse(String(text || ""))
    if (!value || typeof value !== "object") return emptyStatus()
    value.percent = value.progress ? Number(value.progress.percent || 0) : 0
    value.profiles = Array.isArray(value.profiles) ? value.profiles : []
    return value
  } catch (_) { return emptyStatus() }
}

function percent(status) { return Math.max(0, Math.min(100, Math.round(Number(status.percent || 0) * 100))) }
function active(status) { return status.state === "backing-up" || status.state === "checking" }
function hasTotal(status) { return !!status.progress && Number(status.progress.total_files || 0) > 0 }
function lane(status, name) {
  var lanes = Array.isArray(status.profiles) ? status.profiles : []
  for (var i = 0; i < lanes.length; i++) if (lanes[i].profile === name) return lanes[i]
  return {profile: name, state: "idle", percent: 0}
}
function lanePercent(status, name) { return Math.max(0, Math.min(100, Math.round(Number(lane(status, name).percent || 0) * 100))) }
function rate(value) {
  value = Number(value || 0)
  if (value >= 1073741824) return (value / 1073741824).toFixed(1) + " GiB/s"
  if (value >= 1048576) return (value / 1048576).toFixed(1) + " MiB/s"
  if (value >= 1024) return (value / 1024).toFixed(1) + " KiB/s"
  return Math.round(value) + " B/s"
}
function wireRate(status) {
  var value = status.progress ? Number(status.progress.wire_bytes_per_second || 0) : 0
  return rate(value)
}
function hasWireRate(status) { return !!status.progress && Number(status.progress.wire_bytes_per_second || 0) > 0 }
function laneDetail(status, name) {
  var value = lane(status, name)
  var files = Number(value.files || 0)
  var total = Number(value.total_files || 0)
  var detail = files + " / " + total + " files"
  var filesRate = Number(value.files_per_second || 0)
  if (filesRate > 0) detail += "  •  " + (filesRate >= 10 ? Math.round(filesRate) : filesRate.toFixed(1)) + " files/s"
  return detail
}
function hasLane(status, name) {
  var lanes = Array.isArray(status.profiles) ? status.profiles : []
  for (var i = 0; i < lanes.length; i++) if (lanes[i].profile === name) return true
  return false
}
function overdue(status) { return !status.last_healthy || Date.now() - Date.parse(status.last_healthy) >= 7 * 86400000 }
function travel(status) {
  if (!status.travel_until || Date.parse(status.travel_until) <= Date.now()) return "Off"
  return "Muted until " + new Date(status.travel_until).toLocaleString()
}
function resultMark(status) {
  if (status.state === "failed") return " ×"
  if (status.incomplete || overdue(status)) return " !"
  if (status.success_until && Date.parse(status.success_until) > Date.now()) return " ✓"
  return ""
}
function age(iso) {
  if (!iso) return "Never"
  var seconds = Math.max(0, (Date.now() - Date.parse(iso)) / 1000)
  if (seconds < 3600) return Math.floor(seconds / 60) + "m ago"
  if (seconds < 86400) return Math.floor(seconds / 3600) + "h ago"
  return Math.floor(seconds / 86400) + "d ago"
}
