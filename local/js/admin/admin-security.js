// 3D Artefact Exhibition server
// File: admin-security.js, Admin site security script
// Creater: Kevin Mak, October 2021
// (c)Marvel Digital Ltd. 2021

$(document).ready(function () {
  $("#changebtn").on('click', onChangeClicked);
  $("#rolebtn").on('click', onRoleClicked);
  $("#createbtn").on('click', onCreateClicked);
  $("#resetbtn").on('click', onResetClicked);

  // Show/hide password toggles (event delegation so dynamically added toggles work too).
  $(document).on('click', '.pw-toggle', function (e) {
    e.preventDefault();
    const targetId = $(e.currentTarget).data('target');
    const inp = $("#" + targetId);
    if (inp.attr('type') === 'password') {
      inp.attr('type', 'text');
      $(e.currentTarget).text('Hide');
    } else {
      inp.attr('type', 'password');
      $(e.currentTarget).text('Show');
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
        const info = ret.info || "unknown error";
        console.error("userlist failed:", info);
        $("#alertPlace").append(getAlert(false, "User list failed: " + info));
        return;
      }
      const sel = $("#userSelect");
      const rsel = $("#resetUserSelect");
      sel.empty();
      rsel.empty();
      ret.users.forEach(u => {
        const label = u.username + " (" + u.role + ")";
        sel.append($('<option></option>')
          .attr('value', u.id)
          .text(label)
          .data('role', u.role));
        rsel.append($('<option></option>')
          .attr('value', u.id)
          .text(label));
      });
      syncRoleRadios();
      sel.off('change').on('change', syncRoleRadios);
    })
    .fail((jqXHR, textStatus, errorThrown) => {
      const info = (jqXHR.responseJSON && jqXHR.responseJSON.info) ? jqXHR.responseJSON.info : textStatus;
      console.error("userlist request failed:", info);
      $("#alertPlace").append(getAlert(false, "User list failed: " + info));
    });
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
    orgpw: $("#orgpw").val(),
    changepw: $("#changepw").val(),
    changepw2: $("#changepw2").val(),
  };

  let alert = $("#alertPlace");
  alert.empty();
  if (
    data.orgpw.length == 0 ||
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

function onCreateClicked(e) {
  e.preventDefault();
  const data = {
    username: $("#newUsername").val(),
    newpw: $("#newUserpw").val(),
    role: $("input[name='createRoleRadios']:checked").val(),
  };
  let alert = $("#alertPlace");
  alert.empty();
  if (!data.username || !data.newpw || !data.role) {
    alert.append(getAlert(false, "Create fail: fill username, password and role"));
    return;
  }
  $.post("/verify/admin/createuser", data)
    .done(ret => {
      if (ret.msg == "OK") {
        alert.append(getAlert(true, "User created."));
        loadUsers();
        $("#newUsername").val("");
        $("#newUserpw").val("");
      } else {
        alert.append(getAlert(false, "Create fail: " + ret.info));
      }
    })
    .catch(res => {
      alert.append(getAlert(false, "Create fail: " + (res.responseJSON && res.responseJSON.info)));
    });
}

function onResetClicked(e) {
  e.preventDefault();
  const data = {
    userid: $("#resetUserSelect").val(),
    newpw: $("#resetpw").val(),
  };
  let alert = $("#alertPlace");
  alert.empty();
  if (!data.userid || !data.newpw) {
    alert.append(getAlert(false, "Reset fail: select a user and enter a password"));
    return;
  }
  $.post("/verify/admin/resetpw", data)
    .done(ret => {
      if (ret.msg == "OK") {
        alert.append(getAlert(true, "Password reset. User must log in again."));
        $("#resetpw").val("");
      } else {
        alert.append(getAlert(false, "Reset fail: " + ret.info));
      }
    })
    .catch(res => {
      alert.append(getAlert(false, "Reset fail: " + (res.responseJSON && res.responseJSON.info)));
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
