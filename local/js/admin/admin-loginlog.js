// 3D Artefact Exhibition server
// File: admin-loginlog.js, Admin login history script
// (c)Marvel Digital Ltd. 2021

$(document).ready(function () {
  loadLoginLog();
});

// loadLoginLog fetches the full login history from /verify/api/loginrec and
// renders it newest-first. It uses .text() for every cell so usernames/roles
// can't inject HTML.
function loadLoginLog() {
  var place = $("#loginlogPlace");
  place.empty();
  $.get("/verify/api/loginrec", function (data) {
    if (data.msg !== "OK" || !data.data || data.data.length === 0) {
      $("#loginlogEmpty").removeClass("d-none");
      return;
    }
    $("#loginlogEmpty").addClass("d-none");

    // Newest first (the API returns oldest-first).
    var rows = data.data.slice().sort(function (a, b) {
      return (b.time || b.loginTime || 0) - (a.time || a.loginTime || 0);
    });

    rows.forEach(function (e) {
      var ts = e.time || e.loginTime || 0;
      var date = new Date(ts * 1000);
      var timeStr = isNaN(date.getTime())
        ? "-"
        : date.toLocaleDateString() + " " + date.toTimeString();

      place.append(
        $("<tr>").append(
          $("<td>").text(timeStr),
          $("<td>").text(e.username || "-"),
          $("<td>").text(e.role || "-"),
          $("<td>").text(e.id || "-")
        )
      );
    });
  }).fail(function () {
    $("#loginlogEmpty").addClass("d-none");
    place.append(
      $("<tr>").append(
        $("<td colspan='4'>").text("Failed to load login log.")
      )
    );
  });
}
