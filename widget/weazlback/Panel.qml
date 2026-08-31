import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import "Model.js" as Model

Panel {
  id: root
  moduleName: "io.github.bprendie.weazlback"
  ipcTarget: moduleName
  manageIpc: false
  property var anchorItem: null
  property var hostWidget: null
  property string binary: ""
  property var status: Model.emptyStatus()
  property string passphrase: ""
  property bool vaultOpen: false
  property string actionError: ""
  property bool travelMode: false
  property bool cancelConfirmOpen: false
  property bool heavyConfirmOpen: false
  readonly property var barIdentity: hostWidget || root
  readonly property color fg: Color.popups.text
  function open() { controller.show() }
  function close() { controller.hide() }
  function toggle() { opened ? close() : open() }
  function toggleMain() {
    if (opened && !travelMode) close()
    else { travelMode = false; open() }
  }
  function toggleTravel() {
    if (opened && travelMode) close()
    else { travelMode = true; open() }
  }
  function closeForPopoutSwitch() { close() }
  function run(args) {
    if (!action.running) {
      action.command = [binary].concat(args)
      action.running = true
    }
  }
  function backup(profile) {
    if (!vaultOpen || backupAction.running) {
      actionError = "Open the vault first"
      return
    }
    actionError = ""
    backupAction.command = profile === "heavy" ? [binary,"widget","backup","heavy"] : [binary,"widget","backup"]
    backupAction.running = true
  }
  function unlockVault() {
    if (passphrase.length === 0 || unlockAction.running) {
      actionError = "Enter the Vault passphrase"
      return
    }
    actionError = ""
    unlockAction.secret = passphrase
    unlockAction.command = [binary,"widget","agent"]
    unlockAction.running = true
  }
  function refreshVaultStatus() {
    if (!vaultStatusAction.running)
      vaultStatusAction.running = true
  }
  Process { id: action; onExited: if (root.hostWidget) root.hostWidget.refresh() }
  Process {
    id: backupAction
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root.actionError = String(text || "").trim() }
    onExited: function(code, status) {
      if (root.hostWidget) root.hostWidget.refresh()
    }
  }
  Process {
    id: unlockAction
    property string secret: ""
    stdinEnabled: true
    stderr: StdioCollector { waitForEnd: true; onStreamFinished: root.actionError = String(text || "").trim() }
    stdout: SplitParser { onRead: function(line) { if (String(line).trim() === "Vault Open") { root.vaultOpen = true; root.passphrase = ""; root.actionError = "" } } }
    onStarted: { write(secret + "\n"); secret = "" }
    onExited: function(code, status) {
      root.vaultOpen = false
      root.refreshVaultStatus()
    }
  }
  Process {
    id: vaultStatusAction
    command: [root.binary,"widget","vault-status"]
    stdout: SplitParser { onRead: function(line) { root.vaultOpen = String(line).trim() === "Vault Open" } }
  }
  Timer { interval: 1000; running: true; repeat: true; triggeredOnStart: true; onTriggered: root.refreshVaultStatus() }
  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.barIdentity
    bar: root.bar
    open: root.opened
    centerOnBar: true
    contentWidth: panel.fittedContentWidth(Style.space(360))
    contentHeight: panel.fittedContentHeight(content.implicitHeight)
    PanelKeyCatcher { anchors.fill: parent; onCloseRequested: root.close() }
    Column {
      id: content
      anchors.fill: parent
      spacing: Style.space(9)
      Text { text: root.travelMode ? "  TRAVEL MODE" : "  WEAZLBACK"; color: root.fg; font.bold: true; font.pixelSize: Style.font.subtitle }
      Text { visible: !root.travelMode; text: "Last backup  " + Model.age(root.status.last_healthy); color: root.fg }
      Text { visible: !root.travelMode; text: "Destination  " + String(root.status.destination || "Not configured"); color: root.fg }
      Text { visible: !root.travelMode && Model.active(root.status); text: "TOTAL  " + (Model.hasTotal(root.status) ? Model.percent(root.status) + "%" : "discovering …"); color: root.fg }
      Text { visible: !root.travelMode && Model.active(root.status) && Model.hasWireRate(root.status); text: "WIRE  " + Model.wireRate(root.status); color: root.fg }
      Lane { visible: !root.travelMode && Model.active(root.status) && Model.hasLane(root.status,"CORE"); title: "CORE"; percent: Model.lanePercent(root.status,"CORE"); detail: Model.laneDetail(root.status,"CORE"); foreground: root.fg }
      Lane { visible: !root.travelMode && Model.active(root.status) && Model.hasLane(root.status,"HOME"); title: "HOME"; percent: Model.lanePercent(root.status,"HOME"); detail: Model.laneDetail(root.status,"HOME"); foreground: root.fg }
      Lane { visible: !root.travelMode && Model.active(root.status) && Model.hasLane(root.status,"HEAVY"); title: "HEAVY"; percent: Model.lanePercent(root.status,"HEAVY"); detail: Model.laneDetail(root.status,"HEAVY"); foreground: root.fg }
      Text { visible: !root.travelMode && Model.active(root.status) && !root.vaultOpen; text: "Vault locked  •  active backup continues"; color: root.fg }
      TextField {
        visible: !root.travelMode && !root.vaultOpen && !Model.active(root.status)
        anchors.left: parent.left; anchors.right: parent.right
        password: true; placeholderText: "Vault passphrase"; foreground: root.fg
        text: root.passphrase; onTextChanged: root.passphrase = text
        onAccepted: root.unlockVault()
      }
      Button { visible: !root.travelMode && !root.vaultOpen && !Model.active(root.status); text: "Unlock vault"; foreground: root.fg; enabled: !unlockAction.running; onClicked: root.unlockVault() }
      Row {
        visible: !root.travelMode && root.vaultOpen
        spacing: Style.space(8)
        Text { text: "Vault Open"; color: root.fg; font.bold: true }
        Button { text: "Lock vault"; foreground: root.fg; onClicked: { unlockAction.running = false; root.vaultOpen = false; root.passphrase = "" } }
      }
      Text { visible: !root.travelMode && root.actionError !== ""; width: parent.width; wrapMode: Text.Wrap; text: "× " + root.actionError; color: root.fg }
      Button { visible: !root.travelMode; text: "Backup now — Core + Home"; foreground: root.fg; enabled: root.vaultOpen && !backupAction.running && !Model.active(root.status); onClicked: root.backup("routine") }
      Button { visible: !root.travelMode && root.status.cancellable === true; text: root.cancelConfirmOpen ? "Keep backup running" : "Cancel backup…"; foreground: root.fg; onClicked: root.cancelConfirmOpen = !root.cancelConfirmOpen }
      Row {
        id: cancelConfirm; visible: !root.travelMode && root.cancelConfirmOpen; spacing: Style.space(5)
        Text { text: "Gracefully cancel?"; color: root.fg }
        Button { text: "Yes, cancel"; foreground: root.fg; onClicked: { root.cancelConfirmOpen = false; root.run(["widget","cancel"]) } }
      }
      Button { visible: !root.travelMode; text: "Backup Heavy…"; foreground: root.fg; enabled: root.vaultOpen && !backupAction.running && !Model.active(root.status); onClicked: root.heavyConfirmOpen = !root.heavyConfirmOpen }
      Row {
        id: heavyConfirm; visible: !root.travelMode && root.heavyConfirmOpen; spacing: Style.space(5)
        Text { text: "VMs/containers stopped?"; color: root.fg }
        Button { text: "Confirm Heavy"; foreground: root.fg; onClicked: { root.heavyConfirmOpen = false; root.backup("heavy") } }
      }
      Row {
        visible: !root.travelMode; spacing: Style.space(5)
        Button { text: "Open"; foreground: root.fg; onClicked: root.run(["widget","open"]) }
        Button { text: "Restore files"; foreground: root.fg; onClicked: root.run(["widget","restore"]) }
        Button { text: "Check"; foreground: root.fg; onClicked: root.run(["widget","check"]) }
      }
      Text { visible: root.travelMode; text: "Mute reminders only; backups and schedules continue."; color: root.fg; width: parent.width; wrapMode: Text.Wrap }
      Text { visible: root.travelMode; text: Model.travel(root.status); color: root.fg; font.bold: true }
      Row {
        visible: root.travelMode; spacing: Style.space(5)
        Button { text: "1 day"; foreground: root.fg; onClicked: root.run(["widget","travel","1"]) }
        Button { text: "3 days"; foreground: root.fg; onClicked: root.run(["widget","travel","3"]) }
        Button { text: "7 days"; foreground: root.fg; onClicked: root.run(["widget","travel","7"]) }
        Button { text: "Off"; foreground: root.fg; onClicked: root.run(["widget","travel","off"]) }
      }
      Row {
        visible: root.travelMode; spacing: Style.space(5)
        TextField { id: customDays; width: Style.space(110); placeholderText: "Custom days"; foreground: root.fg }
        Button { text: "Set"; foreground: root.fg; onClicked: root.run(["widget","travel",customDays.text]) }
      }
    }
  }
}
