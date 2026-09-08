import pytest
import uuid


def classroom(lab):
    admin,csrf=lab["login"]("admin");teacher,tcsrf=lab["login"]("teacher");student,scsrf=lab["login"]("student")
    mutate=lab["mutate"]
    response=mutate(admin,csrf,"/v3/classes",{"name":"Contextual Class"});assert response.status==200
    cls=response.json();path="/v3/classes/"+cls["id"]
    for subject,role in (("teacher","teacher"),("student","student")):
        assert mutate(admin,csrf,path+"/participants",{"user_id":lab["actors"][subject]["id"],"role":role}).status==200
    return admin,csrf,teacher,tcsrf,student,scsrf,cls,path


@pytest.mark.case("CP12-capability")
def test_teacher_student_management_and_staff_denial(lab):
    admin,csrf,teacher,tcsrf,student,scsrf,cls,path=classroom(lab);m=lab["mutate"]
    assert m(teacher,tcsrf,path+"/participants",{"user_id":lab["actors"]["student2"]["id"],"role":"student"}).status==200
    assert m(teacher,tcsrf,path+"/participants",{"user_id":lab["actors"]["teacher2"]["id"],"role":"teacher"}).status==403
    assert m(teacher,tcsrf,path+"/participants/"+lab["actors"]["teacher"]["id"]+"/end",{}).status==403
    assert m(student,scsrf,path+"/participants",{"user_id":lab["actors"]["student2"]["id"],"role":"student"}).status==403
    assert student.get(path).status==403
    assert m(teacher,tcsrf,path+"/publish",{"version":1}).status==200
    foreign=m(admin,csrf,"/v3/classes",{"name":"Foreign"}).json()
    assert m(teacher,tcsrf,"/v3/classes/"+foreign["id"],{"name":"intrusion","version":1},method="patch").status==403
    assert m(teacher,tcsrf,path+"/close",{"version":2}).status==403


@pytest.mark.case("CP12-lifecycle")
def test_suspend_reactivate_close_reopen_and_history(lab):
    admin,csrf,teacher,tcsrf,student,scsrf,cls,path=classroom(lab);m=lab["mutate"]
    assert m(teacher,tcsrf,path+"/publish",{"version":1}).status==200
    participant=path+"/participants/"+lab["actors"]["student"]["id"]
    suspended=m(teacher,tcsrf,participant+"/suspend",{});assert suspended.status==200 and suspended.json()["status"]=="SUSPENDED"
    assert student.get(path).status==403
    assert m(teacher,tcsrf,participant+"/suspend",{}).status==409
    assert m(teacher,tcsrf,participant+"/reactivate",{}).status==200
    assert student.get(path).status==200
    teacher_path=path+"/participants/"+lab["actors"]["teacher"]["id"]
    assert m(admin,csrf,teacher_path+"/suspend",{}).status==409
    closed=m(admin,csrf,path+"/close",{"version":2});assert closed.status==200 and closed.json()["state"]=="CLOSED"
    assert student.get(path).json()["state"]=="CLOSED"
    assert m(teacher,tcsrf,path,{"name":"closed edit","version":3},method="patch").status==409
    assert m(admin,csrf,path+"/reopen",{"version":2}).status==409
    assert m(admin,csrf,path+"/reopen",{"version":3}).json()["state"]=="PUBLISHED"
    ended=m(teacher,tcsrf,participant+"/end",{});assert ended.status==200
    assert m(teacher,tcsrf,participant+"/reactivate",{}).status==409
    rejoined=m(teacher,tcsrf,path+"/participants",{"user_id":lab["actors"]["student"]["id"],"role":"student"})
    assert rejoined.status==200 and rejoined.json()["enrollment_id"]==ended.json()["enrollment_id"]
    key=uuid.uuid4().hex
    first=m(teacher,tcsrf,participant+"/suspend",{},key=key)
    retry=m(teacher,tcsrf,participant+"/suspend",{},key=key)
    assert first.status==retry.status==200 and first.json()==retry.json()
    assert m(teacher,tcsrf,participant+"/end",{}).status==200
    assert student.get(path).status==403
    archived=m(admin,csrf,path+"/archive",{"version":4})
    assert archived.status==200 and archived.json()["state"]=="ARCHIVED"
    assert m(admin,csrf,path+"/reopen",{"version":5}).status==409


@pytest.mark.case("CP12-prerequisites")
def test_publish_staff_and_cross_context_prerequisites(lab):
    admin,csrf,teacher,tcsrf,student,scsrf,cls,path=classroom(lab);m=lab["mutate"]
    teacher_path=path+"/participants/"+lab["actors"]["teacher"]["id"]
    assert m(admin,csrf,teacher_path+"/suspend",{}).status==200
    assert m(admin,csrf,path+"/publish",{"version":1}).status==409
    assert teacher.get(path).status==403
    assert m(admin,csrf,teacher_path+"/reactivate",{}).status==200
    assert m(admin,csrf,teacher_path+"/reactivate",{}).status==409
    assert m(admin,csrf,path+"/close",{"version":1}).status==409
    assert m(teacher,tcsrf,path+"/publish",{"version":1}).status==200
    second=lab["actors"]["teacher2"]["id"]
    assert m(admin,csrf,path+"/participants",{"user_id":second,"role":"teacher"}).status==200
    second_client,second_csrf=lab["login"]("teacher2")
    assert m(second_client,second_csrf,path,{"name":"Second teacher edit","version":2},method="patch").status==200
    # Student role addition must not enable a Teacher to manage a staff target.
    assert m(admin,csrf,path+"/participants",{"user_id":second,"role":"student"}).status==200
    assert m(teacher,tcsrf,path+"/participants/"+second+"/suspend",{}).status==403
    assert m(admin,csrf,teacher_path+"/suspend",{}).status==200
    assert m(admin,csrf,path+"/participants/"+second+"/end",{}).status==409
    foreign=m(admin,csrf,"/v3/classes",{"name":"Other Class"}).json()
    assert m(teacher,tcsrf,"/v3/classes/"+foreign["id"]+"/participants/"+lab["actors"]["student"]["id"]+"/end",{}).status==403
    assert m(admin,csrf,path+"/participants/"+"f"*32+"/suspend",{}).status==404
    assert m(admin,csrf,path+"/participants",{"user_id":lab["actors"]["disabled"]["id"],"role":"student"}).status==409
