import httpClient from "../utils/http";
import axios from "axios";
import { readFileSync } from "fs";

function getSemesterNameAndYear(id: string) {
  const seasonCode = `${id}`.slice(-2);
  const yearSuffix = id.replace(seasonCode, '');
  const yearRange = `20${yearSuffix}-${parseInt(yearSuffix, 10)+1}`
  switch(seasonCode) {
    case '20':
      return {
        year: yearRange.substring(0,4),
        name: `${yearRange} Winter`
      }
    case '30':
      return {
        year: yearRange.replace(`${yearSuffix}-`, ''),
        name: `${yearRange} Spring`
      }
    case '40':
      return {
        year: yearRange.replace(`${yearSuffix}-`, ''),
        name: `${yearRange} Summer`
      }
    default:
      return {
        year: yearRange.substring(0,4),
        name: `${yearRange} Fall`
      }
  }
}

async function createSemesterIfNotExist(id: number): Promise<void>{
  const { name, year } = getSemesterNameAndYear(id.toString());
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation createSemester($id: bigint!, $name: String!, $year: Int!) {
            createSemester(
              object: {
                id: $id
                name: $name
                year: $year
              }
              on_conflict: {
                constraint: semesters_pkey
                update_columns: [
                  updatedAt
                ]
              }
            ) {
              createdAt
              updatedAt
            }
          }`,
        variables: { id, name, year: parseInt(year, 10) }
      },
    });
    const { createdAt, updatedAt } = data.createSemester;
    console.log(`[!] ${createdAt===updatedAt?`Added new semester "${name}" to semester table`:'No new semester detected, semester table remain unchanged'}`)
  } catch (error) {
    throw error
  }
}

async function addCourse(code: string, semesterId: number) {
  try {
    const { data: { name }} = await axios({
      url: `https://course-quota.now.sh/api/subject?department=COMP&code=${code}`
    });
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation addCourse($code: String!, $semesterId: bigint!, $name: String!) {
            createCourse(
              object: {
                code: $code
                semester_id: $semesterId
                name: $name
              }
            ) {
              id
              semester {
                name
              }
            }
          }
        `,
        variables: {
          code,
          semesterId,
          name
        }
      }
    });
    const { id, semester } = data.createCourse;
    console.log(`[!] Added course ${code} to ${semester.name} semester with id ${id}`);
    return id;
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function addSection(courseId: number, sectionName: string) {
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation addSection($courseId: bigint!, $sectionName: String!) {
            createSection(
              object: {
                course_id: $courseId
                name: $sectionName
              }
            ) {
              id
              name
              course {
                code
              }
            }
          }
        `,
        variables: {
          courseId,
          sectionName
        }
      }
    });
    const { id, name, course } = data.createSection;
    console.log(`[!] Added new section ${name} to course ${course.code} with id ${id}`);
    return id;
  } catch (error) {
    console.error(`[✗] ${error.message}`);
    throw error
  }
}

async function addStudentsToCourseSection(studentUserIds: Array<any>, sectionId: number) {
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation addUsersToSection($users: [section_user_insert_input!]!) {
            addUsersToSection (
              objects: $users
            ) {
              affected_rows
            }
          }

        `,
        variables: {
          users: studentUserIds.map(id => ({user_id: id, section_id: sectionId}))
        }
      }
    });
    const { affected_rows: affectedRows } = data.addUsersToSection;
    return {
      hasDiscrepancy: studentUserIds.length===affectedRows,
      affectedRows
    }
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error;
  }
}

export async function getStudentCourseEnrollmentMap() {
  try {
    const { data: { data, termid, status } } = await axios({
      method: 'post',
      url: process.env.CSSYSTEM_API_URL,
      data: JSON.parse(Buffer.from(process.env.CSSYSTEM_API_SECRET_PAYLOAD, 'base64').toString())
    });
    if(status===0) {
      const records = data.split('\n').filter((line: string) => line.length>0);
      let enrollments: any = [];
      records.forEach((row: string) => {
        const [ itsc, ...courseSectionMaps ] = row.split(',');
	      for(const courseSectionMap of courseSectionMaps) {
	        const [ course, section ] = courseSectionMap.split('-').map(str => str.toUpperCase());
	        enrollments.push({ itsc, course, section });
	      }
      });
      return {
        enrollments,
        semester: parseInt(termid, 10)
      };
    } else {
      throw new Error('error fetching cssystem api');
    }
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function addUsers(unloggedUsers: Array<string>) {
  const users = unloggedUsers.map((itsc: any) => ({ itsc }));
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation registerUsers($users: [users_insert_input!]!) {
            batchCreateUser (
              objects: $users
            ) {
              returning {
                id
                itsc
              }
              affected_rows
            }
          }
        `,
        variables: {
          users
        }
      }
    });
    const { returning, affected_rows } = data.batchCreateUser;
    console.log(`[!] Added ${affected_rows} students to ZINC user table`);
    return returning;
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function addStudentsToCourse(studentIds: Array<any>, courseId: number) {
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation enrollUsersToCourse($users: [course_user_insert_input!]!) {
            enrollUsersInCourse (
              objects: $users
            ) {
              affected_rows
            }
          }
        `,
        variables: {
          users: studentIds.map(id => ({user_id: id, course_id: courseId, permission: 1}))
        }
      }
    });
    const { affected_rows: affectedRows } = data.enrollUsersInCourse;
    return {
      hasDiscrepancy: affectedRows===studentIds.length,
      affectedRows
    };
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function getStudentUserIds(itscIds: Array<string>) {
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          query getUserIds($itscIds: [String!]!) {
            users(where: {
              itsc: {
                _in: $itscIds
              }
            }) {
              id
              itsc
            }
          }
        `,
        variables: {
          itscIds
        }
      }
    });
    const { users } = data;
    const existingUsers = users.map((user: any) => user.itsc);
    const unloggedUsers = itscIds.filter(itscId => !existingUsers.includes(itscId));
    if (unloggedUsers.length > 0) {
      const newlyAddedUsers = await addUsers(unloggedUsers);
      return [...users, ...newlyAddedUsers]
    }
    return users;
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function removeStudentsFromCourse(users: Array<string>) {
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation removeStudentsFromCourse($users: [bigint!]!) {
            removeUsersFromCourse(where: {
              id: {
                _in: $users
              }
            }) {
              affected_rows
            }
          }
        `,
        variables: {
          users
        }
      }
    });
    const { affected_rows: affectedRows } = data.removeUsersFromCourse;
    return {
      hasDiscrepancy: users.length===affectedRows,
      affectedRows
    };
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function removeStudentsFromSection(users: Array<string>) {
  try {
    console.log(users)
    const response = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation removeStudentsFromSection($users: [bigint!]!) {
            removeStudentsFromSection(where: {
              id: {
                _in: $users
              }
            }) {
              affected_rows
            }
          }
        `,
        variables: {
          users
        }
      }
    });
    console.log(response);
    console.log(response.data);
    const { affected_rows: affectedRows } = response.data.data.removeStudentsFromSection;
    return {
      hasDiscrepancy: users.length===affectedRows,
      affectedRows
    }
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

export async function SyncEnrollment() {
  console.log(`[!] Enrollment synchronization begins at ${new Date().toISOString()}`);
  try {
    const { semester, enrollments } = await getStudentCourseEnrollmentMap();
    console.log(`[!] Retrieved ${enrollments.length} enrollment records from CS System`);
    await createSemesterIfNotExist(semester);
    const enrollmentItscs = [...new Set<string>(enrollments.map(({ itsc }: any) => itsc))]
    const students = await getStudentUserIds(enrollmentItscs);
    const enrollmentCourses = [...new Set<string>(enrollments.map((enrollment: any) => enrollment.course))];
    for(const courseCode of enrollmentCourses) {
      const { id: courseId, users, sections } = await getCourse(semester, courseCode);
      const userItscIds = users.map(({user}:any) => user.itsc);
      const studentsToBeEnrolled = enrollments.filter((enrollment: any) => {
        const matchCourse = enrollment.course===courseCode;
        const userDoesNotExist = !userItscIds.includes(enrollment.itsc);
        return userDoesNotExist && matchCourse ;
      });
      const studentsPresentInCourse = enrollments.filter((enrollment: any) => enrollment.course===courseCode).map((record: any) => record.itsc)
      const studentsToBeUnrolled = users.filter((user: any) => !studentsPresentInCourse.includes(user.user.itsc)&&user.permission===1);
      if(studentsToBeUnrolled.length > 0) {
        console.log(JSON.stringify(studentsToBeUnrolled))
        const { hasDiscrepancy, affectedRows } = await removeStudentsFromCourse(studentsToBeUnrolled.map((record: any) => record.id));
        console.log(`[!] Removed ${affectedRows} student${affectedRows>1?'s':''} from course ${courseCode}, ${hasDiscrepancy?'No discrepancy':'Discrepancies'} detected`);
      }
      const enrollingStudentItscIds = studentsToBeEnrolled.map((record: any) => record.itsc);
      const { hasDiscrepancy, affectedRows } = await addStudentsToCourse(students.filter((student: any) => enrollingStudentItscIds.includes(student.itsc)).map((student: any) => student.id), courseId);
      console.log(`[!] Added ${affectedRows} new students to course ${courseCode}, ${hasDiscrepancy?'No discrepancy':'Discrepancies'} detected`);
      const targetSections = [...new Set(studentsToBeEnrolled.map((record: any) => record.section))] as Array<string>;
      for (const targetSection of targetSections) {
        if(!sections.map((section: any) => section.name).includes(targetSection)) {
          const sectionId = await addSection(courseId, targetSection)
          const studentsOfSectionToBeAdded = enrollments.filter((record: any) => record.section===targetSection && record.course===courseCode).map((record: any) => record.itsc);
          const studentIds = students.filter((student: any) => studentsOfSectionToBeAdded.includes(student.itsc)).map((student: any) => student.id);
          const { hasDiscrepancy, affectedRows } = await addStudentsToCourseSection(studentIds, sectionId);
          console.log(`[!] Added ${affectedRows} students to course section ${courseCode} ${targetSection}, ${hasDiscrepancy?'No discrepancy':'Discrepancies'} detected`);
        } else {
          const [currentSection] = sections.filter((section: any) => section.name===targetSection);
          const studentsOfSectionToBeAdded = enrollments.filter((record: any) => {
            const matchCourse = record.course===courseCode
            const matchSection = record.section===currentSection.name;
            const currentSectionUserItscIds = currentSection.users.map(({ user }: any) => user.itsc);
            const userDoesNotExist = !currentSectionUserItscIds.includes(record.itsc);
            return matchCourse && matchSection && userDoesNotExist;
          }).map((record: any) => record.itsc);
          const studentIds = students.filter((student: any) => studentsOfSectionToBeAdded.includes(student.itsc)).map((student: any) => student.id);
          const { hasDiscrepancy, affectedRows } = await addStudentsToCourseSection(studentIds, currentSection.id);
          console.log(`[!] Added ${affectedRows} students to course section ${courseCode} ${targetSection}, ${hasDiscrepancy?'No discrepancy':'Discrepancies'} detected`);
          const studentsPresentInSection = enrollments.filter((enrollment: any) => enrollment.course===courseCode&&enrollment.section===currentSection.name).map((record: any) => record.itsc)
          const { users } = currentSection;
          const studentsToBeRemovedFromSection = users.filter(({ user }: any) => !studentsPresentInSection.includes(user.itsc)).map((user: any) => user.id)
          if (studentsToBeRemovedFromSection.length > 0) {
            const { hasDiscrepancy, affectedRows } = await removeStudentsFromSection(studentsToBeRemovedFromSection);
            console.log(`[!] Removed ${affectedRows} student${affectedRows>1?'s':''} from course section ${courseCode} ${currentSection.name}, ${hasDiscrepancy?'No discrepancy':'Discrepancies'} detected`);
          }
        }
      }
    }
    console.log(`[!] Enrollment synchronization completed at ${new Date().toISOString()}`);
  } catch (error) {
    console.error(`[✗] ${error.message}`);
    throw error;
  }
}

async function getCourse(semesterId: number, courseCode: string): Promise<any> {
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          query getSemesterCourse($semesterId:bigint!, $courseCode: String!) {
            courses(where: {
              code: { _eq: $courseCode }
              semester_id: { _eq: $semesterId }
            }) {
              id
              users {
                id
                permission
                user {
                  itsc
                  id
                }
              }
              sections {
                id
                name
                users {
                  id
                  user {
                    itsc
                    id
                  }
                }
              }
            }
          }`,
        variables: { semesterId, courseCode }
      },
    });
    if (data.courses.length === 0) {
      const id = await addCourse(courseCode, semesterId);
      return {
        id,
        users: [],
        sections: []
      }
    }
    const [ course ] = data.courses;
    return course;
  } catch (error) {
    console.error(`[✗] ${error.message}`);
    throw error;
  }
}

