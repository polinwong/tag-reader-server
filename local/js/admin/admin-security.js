// 3D Artefact Exhibition server
// File: admin-security.js, Admin site security script
// Creater: Kevin Mak, October 2021
// (c)Marvel Digital Ltd. 2021

$(document).ready(function () {
  $("#changebtn").on('click', onChangeClicked);
  $("#rolebtn").on('click', onRoleClicked);

  // Show/hide password toggles.
  $(".pw-toggle").on('click', function () {
    const targetId = $(this).data('target');
    const inp = $("#" + targetId);
    if (inp.attr('type') === 'password') {
      inp.attr('type', 'text');
      $(this).text('Hide');
    } else {
      inp.attr('type', 'password');
      $(this).text('Show');
    }
  });

  // Populate the role-management dropdown for admins.
  if ($("#userSelect").length) {
    loadUsers();
  }
});

function loadUsers() {
  $.getJSON("/verify/admin/userlist")
    .done(ret => {
      if (ret.msg !== "OK") {
        return;
      }
      const sel = $("#userSelect");
      sel.empty();
      ret.users.forEach(u => {
        sel.append($('<option></option>')
          .attr('value', u.id)
          .text(u.username + " (" + u.role + ")")
          .data('role', u.role));
      });
      syncRoleRadios();
      sel.off('change').on('change', syncRoleRadios);
    })
    .fail(() => {/* non-admin: section absent */});
}

function syncRoleRadios() {
  const role = $("#userSelect option:selected").data('role');
  if (role === 'admin') {
    $("#roleAdmin").prop('checked', true);
  } else if (role === 'operator') {
    $("#roleOperator").prop('checked', true);
  }
}

function onChangeClicked(e) {
  e.preventDefault();

  var data = {
    orgid: $("#orgid").val(),
    orgpw: $("#orgpw").val(),
    changeid: $("#changeid").val(),
    changepw: $("#changepw").val(),
    changepw2: $("#changepw2").val(),
  };

  let alert = $("#alertPlace");
  alert.empty();
  if (
    data.orgid.length == 0 ||
    data.orgpw.length == 0 ||
    data.changeid.length == 0 ||
    data.changepw.length == 0 ||
    data.changepw2.length == 0
  ) {
    alert.append(getAlert(false, "Change fail: Info not done"));
    return;
  }

  $.post("/verify/admin/changepw", data)
    .done(ret => {
      if (ret.msg == "OK") {
        alert.append(getAlert(true, "change done. Please log in again."));
        setTimeout(() => { window.location.href = "/verify/"; }, 1500);
      } else {
        alert.append(getAlert(false, "Change fail: " + ret.info));
      }
    })
    .catch(res => {
      alert.append(getAlert(false, "Change fail: " + (res.responseJSON && res.responseJSON.info)));
    });
}

function onRoleClicked(e) {
  e.preventDefault();
  const userId = $("#userSelect").val();
  const role = $("input[name='roleRadios']:checked").val();
  let alert = $("#alertPlace");
  alert.empty();
  if (!userId || !role) {
    alert.append(getAlert(false, "Select a user and a role"));
    return;
  }
  $.post("/verify/admin/changerole", { userid: userId, role: role })
    .done(ret => {
      if (ret.msg == "OK") {
        alert.append(getAlert(true, "Role updated. User must log in again."));
        loadUsers();
      } else {
        alert.append(getAlert(false, "Update fail: " + ret.info));
      }
    })
    .catch(res => {
      alert.append(getAlert(false, "Update fail: " + (res.responseJSON && res.responseJSON.info)));
    });
}

function getAlert (typeSuccess, msg) {
  // alert-success alert-danger
  let alertType = "alert-danger";
  if (typeSuccess) {
    alertType = "alert-success";
  }
  return `<div class="alert fixed-bottom `+alertType+` fade show" role="alert">
  <span>`+msg+`</span>
  <button type="button" class="close float-right" data-dismiss="alert" aria-label="Close">
    <span aria-hidden="true">&times;</span>
  </button>
</div>`;
}
