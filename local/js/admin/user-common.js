// 3D Artefact Exhibition server
// File: user-common.js, user site common script header
// Creater: Kevin Mak, March 2021
// (c)Marvel Digital Ltd. 2021

function renderPagesel (id) {
    var prevfunc = () => {
        if (pageNum == 0)
            return `<li class="page-item disabled"><a class="page-link" href="#" tabindex="-1" aria-disabled="true">Prev</a></li>`;
        else
            return `<li class="page-item"><a class="page-link" href="javascript:changePagePrev()">Prev</a></li>`;
    }
    var nextfunc = () => {
        if (pageNum == pageMax - 1)
            return `<li class="page-item disabled"><a class="page-link" href="#" tabindex="-1" aria-disabled="true">Next</a></li>`;
        else
            return `<li class="page-item"><a class="page-link" href="javascript:changePageNext()">Next</a></li>`;
    }
    var linkobj = $("#"+id);
    linkobj.children().remove();

    var prev = prevfunc();
    $(prev).appendTo(linkobj);
    let skip = false;
    if (pageMax > 4) {
        skip = true;
    }

    let passed = false;
    for (var i = 0; i < pageMax; i++) {
        var obj = "";
        if (pageNum == i) {
            obj = `<li class="page-item active" aria-current="page"><a class="page-link">` + (i + 1) + ` <span class="sr-only">(current)</span></a></li>`;
            passed = false;
        } else {
            if (skip && i > 0 && i < pageMax - 1) {
                obj = `<li class="page-item"><a class="page-link" id="hold">...</a></li>`;
            } else {
                obj = `<li class="page-item"><a class="page-link" href="javascript:changePage('` + i + `');">` + (i + 1) + `</a></li>`
                passed = false;
            }
        }
        
        if (!passed) {
            $(obj).appendTo(linkobj);
        }

        if (obj.search("hold") >= 0)
            passed = true;
    }

    var next = nextfunc();
    $(next).appendTo(linkobj);
}

function changePage (num) {
    pageNum = num;

    renderPagesel();
    renderItems();
}

function changePagePrev () {
    pageNum--;
    changePage(pageNum);
}

function changePageNext () {
    pageNum++;
    changePage(pageNum);
}
