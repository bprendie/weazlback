import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import "Model.js" as Model

BarWidget {
  id: root
  moduleName: "io.github.bprendie.weazlback"
  property var status: Model.emptyStatus()
  property int frame: 0
  readonly property bool active: Model.active(status)
  readonly property var rings: ["◴", "◷", "◶", "◵"]
  readonly property string home: Quickshell.env("HOME") || ""
  readonly property string binary: settings && String(settings.binaryPath || "") !== "" ? String(settings.binaryPath) : home + "/.weazlback/bin/weazlback"
  readonly property bool opened: panelLoader.item ? panelLoader.item.opened : false
  readonly property bool popoutSwitchClosing: panelLoader.item ? panelLoader.item.popoutSwitchClosing : false
  function open() { if (panelLoader.item) panelLoader.item.open() }
  function close() { if (panelLoader.item) panelLoader.item.close() }
  function toggle() { if (panelLoader.item) panelLoader.item.toggleMain() }
  function toggleTravel() { if (panelLoader.item) panelLoader.item.toggleTravel() }
  function closeForPopoutSwitch() { if (panelLoader.item) panelLoader.item.closeForPopoutSwitch() }
  function refresh() { if (!statusProc.running) statusProc.running = true }
  function inject() { if (panelLoader.item) { panelLoader.item.bar = bar; panelLoader.item.anchorItem = button; panelLoader.item.hostWidget = root; panelLoader.item.binary = binary; panelLoader.item.status = status } }
  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight
  Process { id: statusProc; command: [root.binary, "status", "--json"]; stdout: StdioCollector { waitForEnd: true; onStreamFinished: { root.status = Model.parse(text); root.inject() } } }
  Timer { interval: 1000; running: true; repeat: true; triggeredOnStart: true; onTriggered: root.refresh() }
  Process { id: reminderProc; command: [root.binary, "widget", "remind"] }
  Timer { interval: 3600000; running: true; repeat: true; triggeredOnStart: true; onTriggered: if (!reminderProc.running) reminderProc.running = true }
  Timer { interval: 250; running: root.active; repeat: true; onTriggered: root.frame = (root.frame + 1) % 4 }
  Loader { id: panelLoader; active: true; visible: false; source: Qt.resolvedUrl("Panel.qml"); onLoaded: root.inject() }
  WidgetButton {
    id: button; bar: root.bar
    text: root.active ? " " + root.rings[root.frame] + " " + (Model.hasTotal(root.status) ? Model.percent(root.status) + "%" : "…") : "" + Model.resultMark(root.status)
    fontFamily: "CaskaydiaMono Nerd Font"
    onPressed: function(b) {
      if (b === Qt.LeftButton) root.toggle()
      else if (b === Qt.RightButton) root.toggleTravel()
    }
  }
}
