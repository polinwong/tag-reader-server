// 3D Artefact Exhibition server
// File: admin-common.js, Admin site common script header
// Creater: Kevin Mak, March 2021
// (c)Marvel Digital Ltd. 2021

const SS_PLAT_UNKNOWN = 0;
const SS_PLAT_ANDROID = 1;
const SS_PLAT_WINDOW = 2;
const SS_PLAT_IOS = 3;

function getPlatFormTag (platformID) {
    switch (platformID) {
        case SS_PLAT_ANDROID:
            return `<span class="badge badge-primary">Android</span>`;
        case SS_PLAT_WINDOW:
            return `<span class="badge badge-info">Windows</span>`;
        case SS_PLAT_IOS:
            return `<span class="badge badge-secondary">iOS</span>`;
        default:
            return `<span class="badge badge-danger">Unknown</span>`;
    }
}

function showDialogSimple (title, msg) {
    $("#WarningModal .modal-title").html(title);
    $("#WarningModal .modal-body").html(msg);
    $("#WarningModal").modal("show");
}

function logout(e) {
    // Maybe server and reload page action to short make the request droped
    // So we need a 50ms delay to send to logout request
    var timeOutA = setTimeout(function () {
        let i = "/verify";
        let l = "/verify/admin/logout";
        $.post(l, function (data, status) {
            window.location.href = window.location.protocol + "//" + window.location.host + i;
        });
    }, 50); // 1000 would be a second
}