// 3D Artefact Exhibition server
// File: admin-secuirty.js, Admin site security script
// Creater: Kevin Mak, October 2021
// (c)Marvel Digital Ltd. 2021

// import { ajax } from 'jquery';
// import './dev/bootstrap.bundle';

$(document).ready(function () {
  $("#changebtn").on('click', onChangeClicked);
});

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

  //action="/verify/admin/changepw" method="POST"

  $.post("/verify/admin/changepw", data)
    .done(ret => {
      if (ret.msg == "OK") {
        alert.append(getAlert(true, "change done"));
      } else {
        alert.append(getAlert(false, "Change fail: " + ret.info));
      }
    })
    .catch(res => {
      alert.append(getAlert(false, "Change fail: " + res.responseJSON.info));
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