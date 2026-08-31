import QtQuick
import qs.Commons

Column {
  id: root
  property string title: ""
  property int percent: 0
  property string detail: ""
  property color foreground: "white"
  spacing: Style.space(3)
  width: parent ? parent.width : Style.space(300)
  Text { text: root.title + "  " + root.percent + "%"; color: root.foreground }
  Text { visible: root.detail !== ""; text: root.detail; color: root.foreground; opacity: 0.78; font.pixelSize: Style.font.caption }
  Rectangle {
    width: parent.width; height: Style.space(5); radius: height / 2
    color: Qt.rgba(root.foreground.r,root.foreground.g,root.foreground.b,0.18)
    Rectangle { width: parent.width * root.percent / 100; height: parent.height; radius: parent.radius; color: root.foreground }
  }
}
