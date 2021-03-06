import QtQuick 2.4
import QtQuick.Controls 2.1
import QtQuick.Controls.Material 2.1
import QtQuick.Layouts 1.3

ColumnLayout {
    property string name
    property variant messageModel

    MessageList {
        Layout.fillHeight: true
        Layout.fillWidth: true

        id: messagePane
        anchors.margins: 16
        model: messageModel

        headerPositioning: ListView.OverlayHeader

        header: Item {
            z: 2
            width: parent.width
            height: 36

            Label {
                z: 3
                anchors.fill: parent
                anchors.leftMargin: 8
                text: name
                font.pointSize: 15
                font.weight: Font.Light
                verticalAlignment: Label.AlignVCenter
            }

            Pane {
                anchors.fill: parent
                opacity: 0.8
            }
        }
    }
}
