// 3D Artefact Exhibition server
// File: card-verify.js, NFC card tag artefact lib script
// Creater: Kevin Mak, Sep 2021
// (c)Marvel Digital Ltd. 2021

// import { ajax } from 'jquery';
// import './dev/bootstrap.bundle';

var dataset;

var pageNum = -1;
var pageMax = 0;
var pageItems = 0;

const platformColorList = ["danger", "primary", "info", "secondary"];
var topPage = "nav-tab-l1";

var modelListBase = [];
var cardListBase = [];
var loginListBase = [];
var modelInfoModified = false;

var filterModel = "", filterCard = "";
var filterModelSize = 0, filterCardSize = 0;
var filterModelTemp, filterCardTemp;
var searchModelTimeout, searchCardTimeout;
var isSearching = false;
var lastSearchSize = 0;

// ----- Part of startup -----

$(document).ready(function () {
  bsCustomFileInput.init();

  $('a[data-toggle="tab"]').on('shown.bs.tab', function (e) {
    // console.log("target:", e.target);
    pageNum = -1;
    isSearching = false;
    if (e.target.id == "nav-tab-l1") {
      topPage = "nav-tab-l1";
    }
    if (e.target.id == "nav-tab-l2") {
      topPage = "nav-tab-l2";
      modelList(0);
    }
    if (e.target.id == "nav-tab-l3") {
      topPage = "nav-tab-l3";
      cardList(0);
    }
    if (e.target.id == "nav-tab-l4") {
      topPage = "nav-tab-l4";
      loginList();
    }
  });

  $("#cardCustomModal").on("hidden.bs.modal", function () {
    if (!modelInfoModified) {
      return;
    }

    modelInfoModified = false;
    modelListBase = [];
    modelList(pageNum >= 0 ? pageNum : 0);
  });

  let addform = document.forms.namedItem("modelAddForm");
  addform.addEventListener("submit", function (e) {
    postFrom(e, addform);
  });
});

// ----- Part of "Add model data" -----

function onFileUpload(input, id) {
  if (input.files && input.files[0]) {
    var reader = new FileReader();

    reader.onload = function (e) {
      $("#"+id).attr('src', e.target.result);
      $("#"+id).removeClass("hide-obj");
    };

    reader.readAsDataURL(input.files[0]);
  }
  console.log(input);
}

function postFrom(e, f) {
  e.preventDefault();
  let formData = new FormData(f);

  let valName = $("#modelIdName").val();
  let valDesc = $("#modelDesc").val();
  let valImage = $("#modelImg")[0].files[0];
  formData.append("modelIdName", valName);
  formData.append("modelDesc", valDesc);
  formData.append("modelImg", valImage);

  $("#addModelBtn").append(
    `<span id="uploadSpin2" class="ml-3 spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>`
  );

  let alert = $("#alertPlace");
  alert.empty();
  $.ajax({
    url: "/verify/api/modelwrite",
    type: "POST",
    headers: {
      "X-Requested-With": "XMLHttpRequest",
    },
    contentType: false,
    processData: false,
    mimeType: "multipart/form-data",
    data: formData,
  })
    .done(function (ret) {
      let data = JSON.parse(ret);
      $("#uploadSpin2").remove();
      if (data.msg == "OK") {
        alert.append(getAlert(true, "Added model id: " + data.retid));
        f.reset();
        $("#imgPreview").addClass("hide-obj");
      } else {
        alert.append(getAlert(false, data.info));
      }
    })
    .fail(function (ret) {
      $("#uploadSpin2").remove();
      let data = JSON.parse(ret.responseText);
      alert.append(getAlert(false, data.info));
    });
}

// ----- Part of "Model list" -----

function renderModelList (data) {
  let place = $("#modelListPlace");
  place.empty();
  filterModelSize = 0;
  for (let i = 0; i < data.length; i++) {
    let modelData = data[i];
    if (filterModel !== "") {
      let needle = filterModel.toLocaleLowerCase()
      if (modelData.name.toLocaleLowerCase().indexOf(needle) < 0) {
        continue;
      }
    }
    filterModelSize++;

    let modelCard = `<div class="col mb-2"><div class="card">
  <div class="card-body">
    <h5 class="card-title">` + modelData.name + `</h5>`;
    if (modelData.image !== "")
      modelCard += `<img style="max-width: 150px" src="/verify/source/img/` + modelData.image + `" loading="lazy">`;
    modelCard += `<b>Permaweb Links:</b>
    <div>`+ modelData.desc + `</div>
    <div style="font-size: 9px" class="mt-2">`+ modelData.id + `</div>
    <button class="btn btn-secondary" type="button" id="modelOption`+ i + `"
      onclick="deleteModel('`+ modelData.id + `');">
      Delete
    </button>
    <button class="btn btn-secondary" type="button" id="modelEdit`+ i + `"
      onclick="editModel('`+ modelData.id + `');">
      Edit
    </button>
  </div>
</div></div>`;
    place.append(modelCard);
  }
  
  renderPagesel("pageBottomL2");
}

function editModelSubmit() {
  let formData = new FormData();

  let valName = $("#editModelIdName").val();
  let valDesc = $("#editModelDesc").val();
  let valImage = $("#editModelImg")[0].files[0];
  let valId = $("#editModelId").val();
  let hasChanges = valName.trim() !== "" || valDesc.trim() !== "" || valImage != null;
  formData.append("modelIdName", valName);
  formData.append("modelDesc", valDesc);
  formData.append("modelImg", valImage);
  formData.append("modelId", valId);

  $("#modelAddSubmit").append(
    `<span id="uploadSpin3" class="ml-3 spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>`
  );

  let alert = $("#alertPlace");
  alert.empty();
  $.ajax({
    url: "/verify/api/modelwrite",
    type: "POST",
    headers: {
      "X-Requested-With": "XMLHttpRequest",
    },
    contentType: false,
    processData: false,
    mimeType: "multipart/form-data",
    data: formData,
  })
    .done(function (ret) {
      let data = JSON.parse(ret);
      $("#uploadSpin3").remove();
      if (data.msg == "OK") {
        modelInfoModified = hasChanges;
        $("#cardCustomModal").modal("hide");
        alert.append(getAlert(true, "Model information updated."));
      } else {
        alert.append(getAlert(false, data.info));
      }
    })
    .fail(function (ret) {
      $("#uploadSpin3").remove();
      let data = JSON.parse(ret.responseText);
      alert.append(getAlert(false, data.info));
    });
}

function editModel(id) {
  modelInfoModified = false;
  $("#cardTitle").empty();
  $("#cardTitle").append(`Edit model info`);
  $("#cardSubmit").empty();
  $("#cardSubmit").append(
    `<button id="modelAddSubmit" type="button" class="btn btn-primary" onclick="editModelSubmit();">Submit</button>`
  );
  $("#cardInfo").empty();
  $("#cardInfo").append(
    `<form id="modelEditForm" enctype="multipart/form-data" class="container-sm pt-3">
  <div class="mb-3" style="color: #ffac1a;"><i class="mr-2 bi bi-info-circle-fill"></i>Empty input will not change</div>
  <div class="form-group">
    <label for="editModelIdName">Model Name</label>
    <input type="text" class="form-control" id="editModelIdName" aria-describedby="editNameHelp">
    <small id="editNameHelp" class="form-text text-muted">The model name for description</small>
  </div>
  <div class="form-group">
    <label for="editModelDesc">Permaweb Links</label>
    <input type="text" class="form-control" id="editModelDesc" aria-describedby="editModelDescHelp">
    <small id="editModelDescHelp" class="form-text text-muted">The link for future artefact if have</small>
  </div>
  <div class="form-group">
    <div class="custom-file">
      <input type="file" class="custom-file-input" id="editModelImg" accept=".png" onchange="onFileUpload(this, 'editImgPreview');">
      <label class="custom-file-label" for="editModelImg">Model Image</label>
    </div>
    <img class="hide-obj" style="max-height: 256px;" id="editImgPreview" src="#" alt="upload image preview" />
  </div>
  <input type="hidden" id="editModelId" value="`+id+`">
</form>`
  );

  $("#cardCustomModal").modal("show");
}

function deleteModel(id) {
  console.log("delete model:", id);
  $.post("/verify/api/modelwrite", {
    modelDel: "1",
    modelId: id,
  }, function (data, status) {
    let alert = $("#alertPlace");
    alert.empty();
    if (data.msg == "OK") {
      alert.append(getAlert(true, "Model removed"));
      modelListBase = [];
      modelList(0);
    } else {
      alert.append(getAlert(false, data.info));
    }
  })
}

function clearUnlinkImg() {
  $.post("/verify/api/modelimgclean", function (data) {
    let alert = $("#alertPlace");
    alert.empty();
    if (data.msg == "OK") {
      alert.append(getAlert(true, "clean done"));
    } else {
      alert.append(getAlert(true, "clean fail, error: " + data.info))
    }
  });
}

function fillModelList(items) {
  if (items == null) {
    return;
  }
  modelListBase = [];
  for (let i = 0; i < items.length; i++) {
    let modelData = {
      id: items[i].id,
      name: items[i].name,
      desc: items[i].desc,
      image: items[i].image,
    };
    modelListBase.push(modelData);
  }
}

function modelList(page) {
  if (page != null) {
    $.get("/verify/api/modellist?page=" + page, function (data) {
      pageNum = page;
      pageMax = data.maxpage;
      fillModelList(data.items);
      renderModelList(modelListBase);
    });
  } else {
    renderModelList(modelListBase);
  }
}

function serachModelFunc(invar) {
  filterModel = invar.value;
  // console.log("input:", filterText);

  modelList();

  if (filterModel.length > 0 && filterModelSize == 0) {
    isSearching = true;
  }
  if (isSearching && filterModel.length == 0) {
    isSearching = false;
    pageNum = -1;
    modelList(0);
    return;
  }

  clearTimeout(searchModelTimeout);
  searchModelTimeout = null;

  if (isSearching && filterModel.length > 1 && searchModelTimeout == null) {
    searchModelTimeout = setTimeout(function () {
      if (filterModel.length == 0) return;
      $.get("/verify/api/modelsearch?key=" + filterModel, function (data) {
        pageNum = 0;
        pageMax = 0;
        filterModelTemp = modelListBase;
        modelListBase = [];
        fillModelList(data.items);
        renderModelList(modelListBase);
      });
    }, 500);
  }
  lastSearchSize = filterModel.length;
}

function clearModelSearch() {
  $("#serachModel").val("");
  filterModel = "";
  isSearching = false;
  modelList(0);
}

// ----- Part of "Card data" -----

function cardList(page) {
  if (page != null) {      
    $.get("/verify/api/carddata?page=" + page, function (data) {
      if (data.msg == "OK") {
        pageNum = page;
        pageMax = data.maxpage;
        pageItems = data.maxitems;
        cardListBase = data.data;
        cardRenderList();
      } else {
        $("#cardDataPlace").append(`<th scope="row">Error on loading</th>`);
      }
    });
  } else {
    cardRenderList();
  }
}

function cardRenderList() {
  let c = pageNum * pageItems + 1;
  $("#cardDataPlace").empty();
  filterCardSize = 0;
  if (cardListBase == null) return;
  cardListBase.forEach(e => {
    if (filterCard !== "") {
      let needle = filterCard.toLocaleLowerCase()
      if (e.id.toLocaleLowerCase().indexOf(needle) < 0 && e.name.toLocaleLowerCase().indexOf(needle) < 0) {
        return;
      }
    }
    filterCardSize++;

    let status = "";
    switch (e.status) {
      case "NORMAL":
        status = `<span style="color: #21ba45;">NORMAL</span>`;
        break;
      case "JUMP":
        status = `<span style="color: #f2c037;">JUMP</span>`;
        break;
      case "REPEATED":
        status = `<span style="color: #c10015;cursor: pointer;" onclick="cardChecked('` + e.id + `');">REPEATED</span>`;
        break;
    }
    let signBtn = `<a class="px-1" href="#" onclick="relinkCardShow('`+ e.id + `');"><i class="bi-edit bi-link-45deg" title="Re-link to model"></i></a>
    <a class="px-1" href="#" onclick="delCardCustom('`+ e.id + `');"><i class="bi-edit bi-trash-fill"></i></a>
    <a class="px-1" href="#" onclick="changeCardKeyShow('`+ e.id + `', '` + e.fkey + `', '` + e.sign + `');"><i class="bi-edit bi-three-dots-vertical"></i></a>`;
    $("#cardDataPlace").append(`<tr>
<th scope="row">` + c + `</th>
<td>` + e.id + `</td>
<td>` + e.name + ` <span class="modelLinkObj">` + e.link + `<span>` + `</td>
<td>` + e.ctr + ` <span style="font-size:9px;color:#888">(` + (e.ctrhex || "") + `)</span></td>
<td>` + status + `</td>
<td>` + signBtn + `</td>
</tr>`);
    c++;
  });

  renderPagesel("pageBottomL3");
}



function serachCardFunc(invar) {
  if (cardListBase != null && cardListBase.length == 0) return;
  filterCard = invar.value;
  // console.log("input:", filterText);

  cardList();

  if (filterCard.length > 0 && filterCardSize == 0) {
    isSearching = true;
  }

  if (isSearching && filterCard.length == 0) {
    isSearching = false;
    pageNum = -1;
    cardList(0);
    return;
  }
  
  clearTimeout(searchCardTimeout);
  searchCardTimeout = null;

  if (isSearching && filterCard.length > 1 && searchCardTimeout == null) {
    searchCardTimeout = setTimeout(function () {
      if (filterCard.length == 0) return;
      $.get("/verify/api/cardsearch?key=" + filterCard, function (data) {
        pageNum = 0;
        pageMax = 0;
        filterCardTemp = cardListBase;
        cardListBase = data.items;
        cardList();
        searchCardTimeout = null;
      })
    }, 500);
  }
  lastSearchSize = filterCard.length;
}

function cardListShow(id, data) {
  $("#" + id).empty();
  if ($("#" + id).val() != data) {
    $("#" + id).append(data);
  }
}

function cardChecked (v) {
  $.post("/verify/api/cardchecked", {"id": v})
    .done((data) => {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        alert.append(getAlert(true, "Card record updated"));
        cardList();
      } else {
        alert.append(getAlert(false, "Card update failed"));
      }
    })
    .catch(() => {
      $("#alertPlace").append(getAlert(false, "Card update failed"));
    })
}

function changeCardKeyShow (id, orgPw, sign) {
  $("#pwInfo").html(`<form>
  <div class="form-group">
    <label for="cardSign">Signature</label>
    <textarea class="form-control" id="cardSign" style="overflow-wrap: anywhere;" readonly>` + sign + `</textarea>
    <label for="cardKey">Card key</label>
    <input type="text" class="form-control" id="cardKey" value="` + orgPw + `" readonly>
  </div>
  <div class="form-group">
    <label for="currentId">To change card ID</label>
    <input type="text" class="form-control" id="currentId" value="`+id+`" disabled>
  </div>
  <div class="form-group">
    <label for="newPw">Password in hex</label>
    <input type="text" class="form-control" id="newPw" aria-describedby="pwFeedback" oninput="checkInput($('#newPw'), $('#pwSubmit'), 32);">
    <div id="pwFeedback" class="invalid-feedback">
      Password is 32 word in hex value form "0-f"
    </div>
  </div>
</form>`);
  $("#pwEditModal").modal('show');
}

function relinkCardShow (id) {
  if (modelListBase == null || modelListBase.length == 0) {
    $.get("/verify/api/modellist?page=0", function (data) {
      if (data.msg == "OK") fillModelList(data.items);
      relinkCardShow(id);
    });
    return;
  }
  let optionlist = "";
  for (let i = 0; i < modelListBase.length; i++) {
    optionlist += `<option value="` + modelListBase[i].id + `">` + modelListBase[i].name + `</option>`;
  }
  $("#relinkCardId").val(id);
  $("#relinkModelSel").empty();
  $("#relinkModelSel").append(optionlist);
  $("#cardLinkModal").modal("show");
}

function relinkCardSubmit () {
  let id = $("#relinkCardId").val();
  let link = $("#relinkModelSel").val();
  if (id == "" || link == "") return;
  $.post("/verify/api/cardlinkset", { id: id, link: link })
    .done((data) => {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        $("#cardLinkModal").modal("hide");
        alert.append(getAlert(true, "Card re-linked to model"));
        setTimeout(function () { alert.empty(); }, 3000);
        cardList(pageNum); // re-fetch from server so the list updates immediately
      } else {
        $("#cardLinkWarning").empty();
        $("#cardLinkWarning").append(`<span style="color: red;">Card re-link failed</span>`);
      }
    })
    .catch(() => {
      $("#cardLinkWarning").empty();
      $("#cardLinkWarning").append(`<span style="color: red;">Card re-link failed</span>`);
    });
}

function checkInput (input, submit, size) {
  let pw = input.val();
  let hexChk = pw.match(/[0-9abcdef]/g);
  if (pw.length != size || hexChk.length != size) {
    input.addClass("is-invalid");
    input.removeClass("is-valid");
    submit.prop("disabled", true);
  } else {
    input.addClass("is-valid");
    input.removeClass("is-invalid");
    submit.prop("disabled", false);
  }
}

function changeCardKey () {
  var fid = $("#currentId").val();
  var fpw = $("#newPw").val();

  if (fid.length != 14 || fpw.length != 32) {
    return
  }

  $.post("/verify/api/cardpwupdate", { id: fid, pw: fpw })
    .done((data) => {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        $("#pwEditModal").modal("hide");
        alert.append(getAlert(true, "Card password updated"));
        cardList();
      } else {
        $("#pwWarning").empty();
        $("#pwWarning").append(`<span style="color: red;">Card password update failed</span>`);
      }
    })
    .catch(() => {
      $("#pwWarning").empty();
      $("#pwWarning").append(`<span style="color: red;">Card password update failed</span>`);
    });
}

function showCardCustom () {
  $.get("/verify/api/modellist", function (data) {
    let items = data.items;
    let optionlist = ""; 
    for (let i = 0; i < items.length; i++) {
      optionlist += `<option value="` + items[i].id + `">` + items[i].name + `</option>`;
    }

    $("#cardTitle").empty();
    $("#cardTitle").append(`Add custom card data`);
    $("#cardSubmit").empty();
    $("#cardSubmit").append(`<button type="button" class="btn btn-primary" onclick="addCardCustom();">Submit</button>`);
    $("#cardInfo").empty();
    $("#cardInfo").append(`<form>
  <div class="form-group">
    <label for="cNewId">Card ID in base64 URL</label>
    <input type="text" class="form-control" id="cNewId">
  </div>
  <div class="form-group">
    <label for="cNewSign">Sign in base64 URL</label>
    <input type="text" class="form-control" id="cNewSign">
  </div>
  <div class="form-group">
    <label for="cNewLink">Card link</label>
    <select class="form-control" id="cNewLink">`+ optionlist + `
    </select>
  </div>
</form>`);
  
  $("#cardCustomModal").modal("show");
  });
}

function addCardCustom () {
  var fid = $("#cNewId").val();
  var fsign = $("#cNewSign").val();
  var flink = $("#cNewLink").val();

  $.post("/verify/api/cardwrite", { id: fid, sign: fsign, link: flink })
    .done((data) => {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        $("#cardCustomModal").modal("hide");
        alert.append(getAlert(true, "Card added"));
        cardList();
      } else {
        $("#cardWarning").empty();
        $("#cardWarning").append(
          `<span style="color: red;">Card add failed</span>`
        );
      }
    })
    .catch(() => {
      $("#cardWarning").empty();
      $("#cardWarning").append(
        `<span style="color: red;">Card add failed</span>`
      );
    });
}

function delCardCustom (e) {
  $("#cardTitle").empty();
  $("#cardTitle").append(`Del card data`);
  $("#cardSubmit").empty();
  $("#cardSubmit").append(`<button type="button" class="btn btn-danger" onclick="delCardWork('` + e + `');">Delete</button>`);
  $("#cardInfo").empty();
  $("#cardInfo").append(`<form>
  <div class="form-group">
    <label for="cConfirm">Please input card key for confirm</label>
    <input type="text" class="form-control" id="cConfirm">
  </div>
</form>`);
  
  $("#cardCustomModal").modal("show");
}

function delCardWork (fid) {
  var fpw = $("#cConfirm").val();

  $.post("/verify/api/carddel", { id: fid, pw: fpw})
    .done((data) => {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        $("#cardCustomModal").modal("hide");
        alert.append(getAlert(true, "Card deleted"));
        cardList();
      } else {
        $("#cardWarning").empty();
        $("#cardWarning").append(
          `<span style="color: red;">Card delete failed</span>`
        );
      }
    })
    .catch(() => {
      $("#cardWarning").empty();
      $("#cardWarning").append(
        `<span style="color: red;">Card delete failed</span>`
      );
    });
}

// ----- Part of "Login record" -----

function loginList () {
  $.get("/verify/api/loginrec", function (data) {
    if (data.msg == "OK") {
      dataset = data.data;
      console.log(dataset);
      $("#adminRecordPlace").empty();
      dataset.sort(function (a, b) {
        return a["time"] - b["time"];
      })
      dataset.forEach(e => {
        // Create a new JavaScript Date object based on the timestamp
        // multiplied by 1000 so that the argument is in milliseconds, not seconds.
        let date = new Date(e.time * 1000);

        $("#adminRecordPlace").append(
          `<tr><th scope="row">` + e.id + `</th>
  <td>` + date.toLocaleDateString() + " " + date.toTimeString() + `</td>
  <td><a href="#" onclick="removeClicked('`+ e.id + `');"><i class="bi-edit bi-trash-fill"></i></a></td></tr>`
        );
      });
    } else {
      $("#adminRecordPlace").append(`<th scope="row">Error on loading</th>`);
    }
  });
}

function removeClicked (id) {
  $.ajax("/verify/api/logindel/" + id, {
    success: function (data) {
      let alert = $("#alertPlace");
      alert.empty();
      if (data.msg == "OK") {
        alert.append(getAlert(true, "Removed session: " + id));
        loginList();
      } else {
        alert.append(getAlert(false, "Error happen: " + data.info));
      }
    },
    type: "DELETE",
  });
  console.log("removeClicked:", id)
}

// ----- Part of command -----

function renderItems () {
  switch (topPage) {
    // case "nav-tab-l1":
    //   renderAddModel(data);
    //   break;
    case "nav-tab-l2":
      modelList(pageNum);
      break;
    case "nav-tab-l3":
      cardList(pageNum);
      break;
  }
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
