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
    const { data: { title }} = await axios({
      url: `https://api.contrib.ust.dev/v1/courses/${code}`
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
              on_conflict: {
                constraint: courses_code_semester_id_key
                update_columns: updated_at
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
          name: title
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

async function addSections(courseId: number, sectionNames: Array<string>) {
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation batchCreateSection($sections: [sections_insert_input!]!) {
            batchCreateSection(
              objects: $sections
              on_conflict: {
                constraint: sections_course_id_name_key
                update_columns: updated_at
              }
            ) {
              returning {
                id
                name
                course {
                  code
                }
              }
            }
          }
        `,
        variables: {
          sections: sectionNames.map(name => ({ name, course_id: courseId }))
        }
      }
    });
    const { returning } = data.createSection;
    return returning.reduce((acc: any, { id, name, course }: any) => {
      console.log(`[!] Added new section ${name} to course ${course.code} with id ${id}`);
      acc[name] = id;
    }, {});
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

export async function getStudentCourseEnrollmentMap(courseCode: string) {
  try {
    const { data: { access_token }} = await axios({
      method: 'post',
      url: `${process.env.ISO_API_URL}/oauth/token`,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Authorization': `Basic ${Buffer.from([process.env.ISO_API_CLIENT_ID, process.env.ISO_API_CLIENT_SECRET].join(':')).toString('base64')}`
      },
      data: `grant_type=password&username=${process.env.ISO_API_USERNAME}&password=${process.env.ISO_API_PASSWORD}`
    });
    const { data } = await axios({
      method: 'get',
      url: `${process.env.ISO_API_URL}/sis/class_enrl?crseCode=${courseCode}`,
      headers: {
        'authorization': `Bearer ${access_token}`
      }
    })
    return data;
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

async function removeStudentsFromCourse(courseId: number) {
  try {
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation removeStudentsFromCourse($courseId: bigint!) {
            removeUsersFromCourse(where: {
              course_id: {
                _eq: $courseId
              }
              permission: {
                _eq: 1
              }
            }) {
              affected_rows
            }
          }
        `,
        variables: {
          courseId
        }
      }
    });
    const { affected_rows: affectedRows } = data.removeUsersFromCourse;
    return {
      affectedRows
    };
  } catch (error) {
    console.error(`[✗] ${error.message}`)
    throw error
  }
}

async function removeStudentsFromSection(courseId: number) {
  try {
    const response = await httpClient.request({
      url: '/graphql',
      data: {
        query: `
          mutation removeStudentsFromSection($users: [bigint!]!) {
            removeStudentsFromSection(where: {
              section: {
                course_id: {
                  _eq: $courseId
                }
              }
            }) {
              affected_rows
            }
          }
        `,
        variables: {
          courseId
        }
      }
    });
    const { affected_rows: affectedRows } = response.data.data.removeStudentsFromSection;
    return {
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
    for (const course of ['COMP2011', 'COMP2012', 'COMP2211', 'COMP2012H']) {
      const data = await getStudentCourseEnrollmentMap(course);
      await createSemesterIfNotExist(parseInt(data.term, 10));
      const courseId = await addCourse(data.crseCode, parseInt(data.term, 10));
      const sectionNames = data.classes.filter((c : any) => c.classType==='N').map((c: any) => c.section);
      const sections = await addSections(courseId, sectionNames);
      await removeStudentsFromCourse(courseId);
      await removeStudentsFromSection(courseId);
      for (const section of data.classes) {
        const students = await getStudentUserIds(section.students.filter((s: any) => s.enrollStatus==='Enrolled').map((s: any) => s.emailAddr.slice(0, s.emailAddr.indexOf('@'))));
        switch(section.classType) {
          case 'N':
            const sectionId = sections[section.section];
            await addStudentsToCourseSection(students.map((student: any) => student.id), sectionId);
            break;
          case 'E':
            await addStudentsToCourse(students.map((student: any) => student.id), courseId);
            break;
          default:
            console.log(`[!] Skipping section ${section.classType} ${section.section} for course ${section.crseCode}`);
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