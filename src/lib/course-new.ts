import httpClient from "../utils/http";
import axios from "axios";
import { readFileSync } from "fs";

function getSemesterNameAndYear(id: string) {
    const seasonCode = `${id}`.slice(-2);
    const yearSuffix = id.replace(seasonCode, '');
    const yearRange = `20${yearSuffix}-${parseInt(yearSuffix, 10) + 1}`
    switch (seasonCode) {
        case '20':
            return {
                year: yearRange.substring(0, 4),
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
                year: yearRange.substring(0, 4),
                name: `${yearRange} Fall`
            }
    }
}

async function createSemesterIfNotExist(id: number): Promise<void> {
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
                        ){
                            createdAt
                            updatedAt
                        }
                    }
                `,
                variables: { id, name, year: parseInt(year, 10) }
            },
        });
        const { createdAt, updatedAt } = data.createSemester;
        console.log(`[!] ${createdAt === updatedAt ? `Added new semester "${name}" to semester table` : 'No new semester detected, semester table remain unchanged'}`)
    } catch (error) {
        throw error
    }
}

async function addCourse(code: string, semesterId: number) {
    try {
        const { data: { name } } = await axios({
            url: `https://course-quota.now.sh/api/subject?department=COMP&code=${code}`
        });
        const { data: { data } } = await httpClient.request({
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
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `
                    mutation addSection($courseId: bigint!, $sectionName: String!) {
                        createSection(
                            object: {
                                course_id: $courseId
                                name: $sectionName
                            }
                        ){
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
                        ){
                            affected_rows
                        }
                    }
                `,
                variables: {
                    users: studentUserIds.map(id => ({ user_id: id, section_id: sectionId }))
                }
            }
        });
        const { affected_rows: affectedRows } = data.addUsersToSection;
        return {
            hasDiscrepancy: studentUserIds.length === affectedRows,
            affectedRows
        }
    } catch (error) {
        console.error(`[✗] ${error.message}`)
        throw error;
    }
}

export async function getStudentCourseEnrollmentMap() {
    return; // won't work right now without the api secret
    try {
        const { data: { data, termid, status } } = await axios({
            method: 'post',
            url: process.env.CSSYSTEM_API_URL,
            data: JSON.parse(Buffer.from(process.env.CSSYSTEM_API_SECRET_PAYLOAD, 'base64').toString())
        });
        if (status === 0) {
            const records = data.split('\n').filter((line: string) => line.length > 0);
            let enrollments: any = [];
            records.forEach((row: string) => {
                const [itsc, ...courseSectionMaps] = row.split(',');
                for (const courseSectionMap of courseSectionMaps) {
                    const [course, section] = courseSectionMap.split('-').map(str => str.toUpperCase());
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
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `
                    mutation registerUsers($users: [users_insert_input!]!) {
                        batchCreateUser (
                            objects: $users
                        ){
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
                        ){
                            affected_rows
                        }
                    }
                `,
                variables: {
                    users: studentIds.map(id => ({ user_id: id, course_id: courseId, permission: 1 }))
                }
            }
        });
        const { affected_rows: affectedRows } = data.enrollUsersInCourse;
        return {
            hasDiscrepancy: affectedRows === studentIds.length,
            affectedRows
        };
    } catch (error) {
        console.error(`[✗] ${error.message}`)
        throw error
    }
}

async function getStudentUserIds(itscIds: Array<string>) {
    try {
        const { data: { data } } = await httpClient.request({
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
            //return [...users, ...newlyAddedUsers]
            console.log("New users found: " + unloggedUsers);
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
            hasDiscrepancy: users.length === affectedRows,
            affectedRows
        };
    } catch (error) {
        console.error(`[✗] ${error.message}`)
        throw error
    }
}

async function removeStudentsFromSection(users: Array<string>) {
    try {
        const response = await httpClient.request({
            url: '/graphql',
            data: {
                query: `
                    mutation removeStudentsFromSection($users: [bigint!]!) {
                        removeUsersFromSection(where: {
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
        const { affected_rows: affectedRows } = response.data.data.removeUsersFromSection;
        return {
            hasDiscrepancy: users.length === affectedRows,
            affectedRows
        }
    } catch (error) {
        console.error(`[✗] ${error.message}`)
        throw error
    }
}

//export async function SyncEnrollment(){
export async function SyncEnrollment(newDataSrc: any, semester: number = 2110) {
    console.log(`[!] Enrollment synchronization begins at ${new Date().toISOString()}`);

    const oldData = await getOldData();
    //const newData = await getNewData();
    const newData = await getNewData(semester, newDataSrc);

    try {
        const { drops, adds, swaps } = await diff(oldData as EnrolmentData[], newData as EnrolmentData[]); // need function to convert raw data to EnrolmentData
        await unenrol(drops);
        await enrol(adds);
        await swap(swaps);
    } catch (error) {
        console.error(`[✗] An error occured during enrollment sync!! ${error.message}`);
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
                    }
                `,
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
        const [course] = data.courses;
        return course;
    } catch (error) {
        console.error(`[✗] ${error.message}`);
        throw error;
    }
}

// New

async function getNewData(semester: number, enrollments: any): Promise<EnrolmentData[]> {
    /*
    // retrieve enrollment data from CSSystem
    const { semester, enrollments } = await getStudentCourseEnrollmentMap();
    console.log(`[!] Retrieved ${enrollments.length} enrollment records from CS System`);
    */
    // create semester if not exist in database
    await createSemesterIfNotExist(semester);
    // convert retrieved ITSCs to Student IDs
    const sids: Map<string, string> = new Map<string, string>();
    (await getStudentUserIds(enrollments.map(({ itsc }: any) => itsc)) as Array<any>).forEach(({ id, itsc }) => sids.set(itsc, id));
    // extract a list of courses
    const courses = [...new Set<string>(enrollments.map(({ course }: any) => course))];
    // transform into a workable format
    const e = enrollments.map((enrollment: any) => ({
        sid: sids.get(enrollment.itsc)!,
        semester: semester,
        course: enrollment.course,
        section: enrollment.section,
    }));

    return e;
}

async function getOldData(): Promise<EnrolmentData[]> {
    //async function getOldData(enrollments: any): Promise<EnrolmentData[]> {
    try {
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `query getUsersCourses {
                    users {
                        id
                        section_users {
                            section {
                                name
                                course {
                                    code
                                    semester_id
                                }
                            }
                        }
                    }
                }`
            },
        });
        //const { data: { data } } = enrollments;

        const users: EnrolmentData[] = [];
        data.users.forEach((user: any) => {
            user.section_users.forEach((section_user: any) => {
                users.push(
                    {
                        sid: user.id,
                        semester: section_user.section.course.semester_id,
                        course: section_user.section.course.code,
                        section: section_user.section.name,
                    }
                );
            });
        });

        return users;
    } catch (error: any) {
        console.error(`[✗] ${error.message}`);
        throw error;
    }
}
async function getSectionByName(semesterId: number, courseCode: string, sectionName: string): Promise<any> {
    try {
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `query getSectionByName($semesterId:bigint!, $courseCode: String!, $sectionName: String!) {
                    sections(where: {
                        course: {
                            code: { _eq: $courseCode },
                            semester_id: { _eq: $semesterId }
                        },
                        name: { _eq: $sectionName }
                    }) {
                        id
                        name
                        course_id
                    }
                }`,
                variables: { semesterId, courseCode, sectionName }
            },
        });
        if (data.sections.length === 0) {
            const courseId = (await getCourse(semesterId, courseCode)).id;
            const id = await addSection(courseId, sectionName);
            return {
                id,
                name: sectionName,
                course_id: courseId
            }
        }
        return data.sections;
    } catch (error: any) {
        console.error(`[✗] ${error.message}`);
        throw error;
    }
}
async function getCourseUserById(userId: number, courseId: number): Promise<any> {
    try {
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `query getSectionUserById($userId: bigint!, $courseId:bigint!) {
                    course_user(where:{
                        user_id: {_eq: $userId},
                        course_id: {_eq: $courseId}
                    }) {
                        id
                    }
                }`,
                variables: { userId, courseId }
            },
        });
        return data.course_user[0];
    } catch (error: any) {
        console.error(`[✗] ${error.message}`);
        throw error;
    }
}
async function getSectionUserById(userId: number, sectionId: number): Promise<any> {
    try {
        const { data: { data } } = await httpClient.request({
            url: '/graphql',
            data: {
                query: `query getSectionUserById($userId:bigint!, $sectionId:bigint!) {
                    section_user(where:{
                        user_id: {_eq: $userId},
                        section_id: {_eq: $sectionId}
                    }) {
                        id
                    }
                }`,
                variables: { userId, sectionId }
            },
        });
        return data.section_user[0];
    } catch (error: any) {
        console.error(`[✗] ${error.message}`);
        throw error;
    }
}
/* End of API calls declaration */

/* Utilities */
interface EnrolmentData {
    sid: string;
    semester: number;
    course: string;
    section: string;
}

function sortBySidThenCourse(a: EnrolmentData, b: EnrolmentData): number {
    const sameSid = String(a.sid).localeCompare(b.sid);
    return sameSid == 0 ? a.course.localeCompare(b.course) : sameSid;
}

function filterLabs(d: EnrolmentData) {
    return d.section.startsWith("LA");
}

function toEnrolmentMap(data: EnrolmentData[]): Map<string, string> {
    const map: Map<string, string> = new Map();
    data.forEach((d) => {
        const key: string = [d.sid, d.semester, d.course].toString();
        const value: string = d.section;
        map.set(key, value);
    });
    return map;
}

function extractKey(enrolmentMapKey: string): [string, string, string] {
    return [...enrolmentMapKey.split(",")] as [string, string, string];
}

/* Procedures */

export function diff(oldData: EnrolmentData[], newData: EnrolmentData[]) {

    // transform to a format we can work with
    const oldEnrolmentMap = toEnrolmentMap(oldData.sort(sortBySidThenCourse).filter(filterLabs)); // let's filter lab courses so I don't destroy the db
    const newEnrolmentMap = toEnrolmentMap(newData.sort(sortBySidThenCourse).filter(filterLabs));
    //const oldEnrolmentMap = toEnrolmentMap(oldData.sort(sortBySidThenCourse)); // let's not filter non lab courses for now because that didn't happen in old algo
    //const newEnrolmentMap = toEnrolmentMap(newData.sort(sortBySidThenCourse));

    // get drops
    const drops: Map<string, string> = new Map();
    const dropsKeys = [...oldEnrolmentMap.keys()].filter((k) => !newEnrolmentMap.has(k));
    dropsKeys.forEach((k) => {
        drops.set(k, oldEnrolmentMap.get(k)!);
        oldEnrolmentMap.delete(k);
    });

    // get adds
    const adds: Map<string, string> = new Map();
    const addsKeys = [...newEnrolmentMap.keys()].filter((k) => !oldEnrolmentMap.has(k));
    addsKeys.forEach((k) => {
        adds.set(k, newEnrolmentMap.get(k)!);
        newEnrolmentMap.delete(k);
    });

    // log drops & adds
    console.log("Drops: ");
    console.log(drops);
    console.log("Adds: ");
    console.log(adds);

    // get swaps
    if (newEnrolmentMap.size != oldEnrolmentMap.size) throw Error(`Inconsistent data sets detected, new map has size ${newEnrolmentMap.size} while old had ${oldEnrolmentMap.size}`);
    const swaps: Map<string, string> = new Map();
    const swapsKeys = [...newEnrolmentMap.keys()].filter((key) => oldEnrolmentMap.get(key) != newEnrolmentMap.get(key));
    swapsKeys.forEach((k) => swaps.set(k, `${oldEnrolmentMap.get(k)!}->${newEnrolmentMap.get(k)!}`));

    // log swaps
    console.log("Swaps: ");
    console.log(swaps)

    // return the data
    return { drops, adds, swaps };

}

async function enrol(data: Map<string, string>) { // LGTM
    // API calls
    data.forEach(async (v, k) => {
        const [sid, semester, course] = extractKey(k), section = v;
        console.log(sid, semester, course, section);

        const courseData = await getCourse(parseInt(semester), course);
        const courseId = courseData.id || courseData[0].id;
        console.log(courseId);
        const sectionData = await getSectionByName(parseInt(semester), course, section);
        const sectionId = sectionData.id || sectionData[0].id;
        console.log(sectionId);

        await addStudentsToCourse([sid], courseId);
        await addStudentsToCourseSection([sid], sectionId);
    });
}

async function unenrol(data: Map<string, string>) {
    // API calls
    data.forEach(async (v, k) => {
        const [sid, semester, course] = extractKey(k), section = v;
        console.log(sid, semester, course, section);

        const sectionData = await getSectionByName(parseInt(semester), course, section);
        const sectionId = sectionData.id || sectionData[0].id;
        const courseId = sectionData.course_id || sectionData[0].course_id;
        console.log(courseId + " " + sectionId);

        const sectionUser = await getSectionUserById(parseInt(sid), sectionId);
        const removedSectionRows = (await removeStudentsFromSection([sectionUser.id])).affectedRows;
        console.log("Section removal results: ");
        console.log(removedSectionRows);

        const courseUser = await getCourseUserById(parseInt(sid), courseId);
        const removedCourseRows = (await removeStudentsFromCourse([courseUser.id])).affectedRows;
        console.log("Course removal results: ");
        console.log(removedCourseRows);
    });

}

async function swap(data: Map<string, string>) {
    // API calls
    data.forEach(async (v, k) => {
        const [sid, semester, course] = extractKey(k), [oldSection, newSection] = v.split("->");
        console.log(sid, semester, course, oldSection, newSection);

        const oldSectionData = await getSectionByName(parseInt(semester), course, oldSection);
        const oldSectionId = oldSectionData.id || oldSectionData[0].id;
        const newSectionData = await getSectionByName(parseInt(semester), course, newSection);
        const newSectionId = newSectionData.id || newSectionData[0].id;

        const sectionUser = await getSectionUserById(parseInt(sid), oldSectionId);

        await removeStudentsFromSection([sectionUser.id]);
        await addStudentsToCourseSection([sid], newSectionId);
    });

}
